// Package notilens provides the NotiLens SDK for sending notifications
// from AI agents, background jobs, and any Go application.
package notilens

import (
	"fmt"
	"os"
	"time"
)

// NotiLens is the main SDK client.
type NotiLens struct {
	agent   string
	token   string
	secret  string
	metrics map[string]interface{}
}

// Options for Init.
type Options struct {
	Token  string
	Secret string
}

// Init creates a NotiLens instance.
// Credentials are resolved in order: Options → env vars → saved CLI config.
func Init(agent string, opts ...Options) (*NotiLens, error) {
	var token, secret string
	if len(opts) > 0 {
		token  = opts[0].Token
		secret = opts[0].Secret
	}
	if token  == "" { token  = os.Getenv("NOTILENS_TOKEN")  }
	if secret == "" { secret = os.Getenv("NOTILENS_SECRET") }
	if token == "" || secret == "" {
		if conf, ok := GetAgent(agent); ok {
			if token  == "" { token  = conf.Token  }
			if secret == "" { secret = conf.Secret }
		}
	}
	if token == "" || secret == "" {
		return nil, fmt.Errorf(
			"NotiLens: token and secret are required. Pass them directly, "+
				"set NOTILENS_TOKEN/NOTILENS_SECRET env vars, or run: "+
				"notilens init --agent %s --token TOKEN --secret SECRET", agent,
		)
	}
	return &NotiLens{
		agent:   agent,
		token:   token,
		secret:  secret,
		metrics: map[string]interface{}{},
	}, nil
}

// ── Metrics ───────────────────────────────────────────────────────────────────

// Metric sets a metric. Numeric values accumulate; strings are replaced.
func (n *NotiLens) Metric(key string, value interface{}) *NotiLens {
	if fv, ok := toFloat64(value); ok {
		if existing, exists := n.metrics[key]; exists {
			if fe, ok := toFloat64(existing); ok {
				n.metrics[key] = fe + fv
				return n
			}
		}
	}
	n.metrics[key] = value
	return n
}

// ResetMetrics resets one metric by key, or all metrics if no key given.
func (n *NotiLens) ResetMetrics(key ...string) *NotiLens {
	if len(key) > 0 {
		delete(n.metrics, key[0])
	} else {
		n.metrics = map[string]interface{}{}
	}
	return n
}

// ── Task lifecycle ────────────────────────────────────────────────────────────

// TaskStart starts a task and returns the task ID.
func (n *NotiLens) TaskStart(taskID ...string) string {
	id := ""
	if len(taskID) > 0 && taskID[0] != "" {
		id = taskID[0]
	} else {
		id = fmt.Sprintf("task_%d", time.Now().UnixMilli())
	}
	sf := GetStateFile(n.agent, id)
	WriteState(sf, map[string]interface{}{
		"agent":       n.agent,
		"task":        id,
		"start_time":  time.Now().UnixMilli(),
		"retry_count": 0,
		"loop_count":  0,
	})
	n.send("task.started", "Task started", id, nil)
	return id
}

// TaskProgress sends a progress update for a running task.
func (n *NotiLens) TaskProgress(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.progress", message, taskID, nil)
}

// TaskLoop signals a loop iteration.
func (n *NotiLens) TaskLoop(message, taskID string) {
	sf    := GetStateFile(n.agent, taskID)
	state := ReadState(sf)
	count := toInt(state["loop_count"]) + 1
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "loop_count": count})
	n.send("task.loop", message, taskID, nil)
}

// TaskRetry signals a retry attempt.
func (n *NotiLens) TaskRetry(taskID string) {
	sf    := GetStateFile(n.agent, taskID)
	state := ReadState(sf)
	count := toInt(state["retry_count"]) + 1
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "retry_count": count})
	n.send("task.retry", "Retrying task", taskID, nil)
}

// TaskError signals a non-fatal error — the task continues.
func (n *NotiLens) TaskError(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "last_error": message})
	n.send("task.error", message, taskID, nil)
}

// TaskComplete signals successful completion (terminal — clears state).
func (n *NotiLens) TaskComplete(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.completed", message, taskID, nil)
	DeleteState(sf)
}

// TaskFail signals a terminal failure.
func (n *NotiLens) TaskFail(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.failed", message, taskID, nil)
	DeleteState(sf)
}

// TaskTimeout signals a terminal timeout.
func (n *NotiLens) TaskTimeout(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.timeout", message, taskID, nil)
	DeleteState(sf)
}

// TaskCancel signals a terminal cancellation.
func (n *NotiLens) TaskCancel(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.cancelled", message, taskID, nil)
	DeleteState(sf)
}

// TaskStop signals a non-terminal stop.
func (n *NotiLens) TaskStop(taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.stopped", "Task stopped", taskID, nil)
}

// TaskTerminate signals a terminal force-termination.
func (n *NotiLens) TaskTerminate(message, taskID string) {
	sf := GetStateFile(n.agent, taskID)
	UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
	n.send("task.terminated", message, taskID, nil)
	DeleteState(sf)
}

// ── Input events ──────────────────────────────────────────────────────────────

// InputRequired signals that human input is needed.
func (n *NotiLens) InputRequired(message, taskID string) {
	n.send("input.required", message, taskID, nil)
}

// InputApproved signals that input was approved.
func (n *NotiLens) InputApproved(message, taskID string) {
	n.send("input.approved", message, taskID, nil)
}

// InputRejected signals that input was rejected.
func (n *NotiLens) InputRejected(message, taskID string) {
	n.send("input.rejected", message, taskID, nil)
}

// ── Output events ─────────────────────────────────────────────────────────────

// OutputGenerated signals that output was produced (AI response, report, file, etc.).
func (n *NotiLens) OutputGenerated(message, taskID string) {
	n.send("output.generated", message, taskID, nil)
}

// OutputFailed signals that output generation failed.
func (n *NotiLens) OutputFailed(message, taskID string) {
	n.send("output.failed", message, taskID, nil)
}

// ── Generic emit ──────────────────────────────────────────────────────────────

// EmitOptions holds optional parameters for Emit.
type EmitOptions struct {
	Meta  map[string]interface{}
	Level string
}

// Emit sends a free-form event.
func (n *NotiLens) Emit(event, message string, opts ...EmitOptions) {
	meta := map[string]interface{}{}
	if len(opts) > 0 && opts[0].Meta != nil {
		meta = opts[0].Meta
	}
	n.send(event, message, "", meta)
}

// ── Internal send ─────────────────────────────────────────────────────────────

func (n *NotiLens) send(event, message, taskID string, meta map[string]interface{}) {
	title := n.agent + " | " + event
	if taskID != "" {
		title = n.agent + " | " + taskID + " | " + event
	}

	// Read state for duration / counts
	var duration, retryCount, loopCount int64
	if taskID != "" {
		sf    := GetStateFile(n.agent, taskID)
		state := ReadState(sf)
		duration   = toInt64(state["duration_ms"])
		retryCount = toInt64(state["retry_count"])
		loopCount  = toInt64(state["loop_count"])
	}

	// Build meta
	extraMeta := map[string]interface{}{"agent": n.agent}
	for k, v := range meta {
		extraMeta[k] = v
	}
	if duration   > 0 { extraMeta["duration_ms"]  = duration   }
	if retryCount > 0 { extraMeta["retry_count"]   = retryCount }
	if loopCount  > 0 { extraMeta["loop_count"]    = loopCount  }
	for k, v := range n.metrics {
		extraMeta[k] = v
	}

	// Strip reserved URL/tag fields from extraMeta into top-level
	imageURL    := popString(extraMeta, "image_url")
	openURL     := popString(extraMeta, "open_url")
	downloadURL := popString(extraMeta, "download_url")
	tags        := popString(extraMeta, "tags")

	isActionable := actionableEvents[event]
	if v, ok := extraMeta["is_actionable"]; ok {
		if b, ok := v.(bool); ok {
			isActionable = b
		}
		delete(extraMeta, "is_actionable")
	}

	payload := map[string]interface{}{
		"event":         event,
		"title":         title,
		"message":       message,
		"type":          getEventType(event),
		"agent":         n.agent,
		"task_id":       taskID,
		"is_actionable": isActionable,
		"image_url":     imageURL,
		"open_url":      openURL,
		"download_url":  downloadURL,
		"tags":          tags,
		"ts":            float64(time.Now().UnixMilli()) / 1000.0,
		"meta":          extraMeta,
	}

	// Silent fail — matches other SDK behaviour
	_ = sendHTTP(n.token, n.secret, payload)
}

func popString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		delete(m, key)
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
