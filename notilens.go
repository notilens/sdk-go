// Package notilens provides the NotiLens SDK for sending notifications
// from AI agents, background jobs, and any Go application.
package notilens

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// NotiLens is the main SDK client.
type NotiLens struct {
	agent    string
	token    string
	secret   string
	stateTtl int
	metrics  map[string]interface{}
	sender   chan map[string]interface{}
}

// Options for Init.
type Options struct {
	Token    string
	Secret   string
	StateTtl int // orphaned state TTL in seconds (default: 86400)
}

// Init creates a NotiLens instance.
// Credentials are resolved in order: Options → env vars → saved CLI config.
func Init(agent string, opts ...Options) (*NotiLens, error) {
	var token, secret string
	stateTtl := 86400
	if len(opts) > 0 {
		token  = opts[0].Token
		secret = opts[0].Secret
		if opts[0].StateTtl > 0 {
			stateTtl = opts[0].StateTtl
		}
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
	CleanupStaleState(agent, stateTtl)

	// Single background worker — one goroutine shared across all sends
	sender := make(chan map[string]interface{}, 256)
	go func() {
		for payload := range sender {
			_ = sendHTTP(token, secret, payload)
		}
	}()

	return &NotiLens{
		agent:    agent,
		token:    token,
		secret:   secret,
		stateTtl: stateTtl,
		metrics:  map[string]interface{}{},
		sender:   sender,
	}, nil
}

// ── Task factory ──────────────────────────────────────────────────────────────

// Task creates a new Run for the given label.
// Each call generates a unique run_id — concurrent executions never conflict.
func (n *NotiLens) Task(label string) *Run {
	CleanupStaleState(n.agent, n.stateTtl)
	runId := genRunId()
	return &Run{
		agent:   n,
		label:   label,
		RunId:   runId,
		sf:      GetStateFile(n.agent, runId),
		metrics: map[string]interface{}{},
	}
}

// ── Agent-level metrics ───────────────────────────────────────────────────────

// Metric sets a metric on the agent. Numeric values accumulate; strings are replaced.
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

// ── Generic track ─────────────────────────────────────────────────────────────

// TrackOptions holds optional parameters for Track.
type TrackOptions struct {
	Meta  map[string]interface{}
	Level string
}

// Track sends a free-form agent-level event.
func (n *NotiLens) Track(event, message string, opts ...TrackOptions) {
	meta := map[string]interface{}{}
	if len(opts) > 0 && opts[0].Meta != nil {
		meta = opts[0].Meta
	}
	n.sendPayload(event, message, "", "", "", n.metrics, meta, "info")
}

// ── Internal send ─────────────────────────────────────────────────────────────

func (n *NotiLens) sendPayload(
	event, message, runId, label, stateFile string,
	runMetrics map[string]interface{},
	extraMeta map[string]interface{},
	level string,
) {
	title := n.agent + " | " + event
	if label != "" {
		title = n.agent + " | " + label + " | " + event
	}

	meta := map[string]interface{}{"agent": n.agent}
	if runId != "" {
		meta["run_id"] = runId
	}
	if label != "" {
		meta["task"] = label
	}

	// Compute duration fields from state
	if stateFile != "" {
		state := ReadState(stateFile)
		now        := time.Now().UnixMilli()
		startTime  := toInt64(state["start_time"])
		queuedAt   := toInt64(state["queued_at"])
		pauseTotal := toInt64(state["pause_total_ms"])
		waitTotal  := toInt64(state["wait_total_ms"])
		if v := toInt64(state["paused_at"]); v > 0 { pauseTotal += now - v }
		if v := toInt64(state["wait_at"]);   v > 0 { waitTotal  += now - v }
		totalMs := int64(0)
		if startTime > 0 { totalMs = now - startTime }
		queueMs := int64(0)
		if startTime > 0 && queuedAt > 0 { queueMs = startTime - queuedAt }
		activeMs := totalMs - pauseTotal - waitTotal
		if activeMs < 0 { activeMs = 0 }

		if totalMs   > 0 { meta["total_duration_ms"] = totalMs   }
		if queueMs   > 0 { meta["queue_ms"]          = queueMs   }
		if pauseTotal > 0 { meta["pause_ms"]         = pauseTotal }
		if waitTotal  > 0 { meta["wait_ms"]          = waitTotal  }
		if activeMs  > 0 { meta["active_ms"]         = activeMs  }
		if v := toInt64(state["retry_count"]); v > 0 { meta["retry_count"] = v }
		if v := toInt64(state["loop_count"]);  v > 0 { meta["loop_count"]  = v }
		if v := toInt64(state["error_count"]); v > 0 { meta["error_count"] = v }
		if v := toInt64(state["pause_count"]); v > 0 { meta["pause_count"] = v }
		if v := toInt64(state["wait_count"]);  v > 0 { meta["wait_count"]  = v }
		if m, ok := state["metrics"]; ok {
			if mm, ok := m.(map[string]interface{}); ok {
				for k, v := range mm {
					meta[k] = v
				}
			}
		}
	}

	for k, v := range runMetrics {
		meta[k] = v
	}
	for k, v := range extraMeta {
		meta[k] = v
	}

	// Strip reserved URL/tag fields from meta into top-level
	imageURL    := popString(meta, "image_url")
	openURL     := popString(meta, "open_url")
	downloadURL := popString(meta, "download_url")
	tags        := popString(meta, "tags")

	isActionable := actionableEvents[event]
	if v, ok := meta["is_actionable"]; ok {
		if b, ok := v.(bool); ok {
			isActionable = b
		}
		delete(meta, "is_actionable")
	}

	payload := map[string]interface{}{
		"event":         event,
		"title":         title,
		"message":       message,
		"type":          getEventType(event),
		"agent":         n.agent,
		"task_id":       label,
		"is_actionable": isActionable,
		"image_url":     imageURL,
		"open_url":      openURL,
		"download_url":  downloadURL,
		"tags":          tags,
		"ts":            float64(time.Now().UnixMilli()) / 1000.0,
		"meta":          meta,
	}

	// Fire-and-forget — push to background worker queue, never blocks the caller
	select {
	case n.sender <- payload:
	default: // channel full — drop
	}
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

// ── Run ───────────────────────────────────────────────────────────────────────

// Run is an isolated execution context for a single task invocation.
type Run struct {
	agent   *NotiLens
	label   string
	RunId   string
	sf      string
	metrics map[string]interface{}
}

// ── Run metrics ───────────────────────────────────────────────────────────────

// Metric sets a metric on the run. Numeric values accumulate; strings are replaced.
func (r *Run) Metric(key string, value interface{}) *Run {
	if fv, ok := toFloat64(value); ok {
		if existing, exists := r.metrics[key]; exists {
			if fe, ok := toFloat64(existing); ok {
				r.metrics[key] = fe + fv
				return r
			}
		}
	}
	r.metrics[key] = value
	return r
}

// ResetMetrics resets one metric by key, or all metrics if no key given.
func (r *Run) ResetMetrics(key ...string) *Run {
	if len(key) > 0 {
		delete(r.metrics, key[0])
	} else {
		r.metrics = map[string]interface{}{}
	}
	return r
}

// ── Run lifecycle ─────────────────────────────────────────────────────────────

// Queue signals that the task has been queued.
func (r *Run) Queue() *Run {
	WriteState(r.sf, map[string]interface{}{
		"agent":          r.agent.agent,
		"task":           r.label,
		"run_id":         r.RunId,
		"queued_at":      time.Now().UnixMilli(),
		"retry_count":    0,
		"loop_count":     0,
		"error_count":    0,
		"pause_count":    0,
		"wait_count":     0,
		"pause_total_ms": 0,
		"wait_total_ms":  0,
	})
	r.send("task.queued", "Task queued", nil, "info")
	return r
}

// Start begins execution of the run.
func (r *Run) Start() *Run {
	now      := time.Now().UnixMilli()
	existing := ReadState(r.sf)
	if len(existing) > 0 {
		UpdateState(r.sf, map[string]interface{}{"start_time": now})
	} else {
		WriteState(r.sf, map[string]interface{}{
			"agent":          r.agent.agent,
			"task":           r.label,
			"run_id":         r.RunId,
			"start_time":     now,
			"retry_count":    0,
			"loop_count":     0,
			"error_count":    0,
			"pause_count":    0,
			"wait_count":     0,
			"pause_total_ms": 0,
			"wait_total_ms":  0,
		})
	}
	r.send("task.started", "Task started", nil, "info")
	return r
}

// Progress sends a progress update.
func (r *Run) Progress(message string) {
	r.send("task.progress", message, nil, "info")
}

// Loop signals a loop iteration.
func (r *Run) Loop(message string) {
	state := ReadState(r.sf)
	UpdateState(r.sf, map[string]interface{}{"loop_count": toInt(state["loop_count"]) + 1})
	r.send("task.loop", message, nil, "info")
}

// Retry signals a retry attempt.
func (r *Run) Retry() {
	state := ReadState(r.sf)
	UpdateState(r.sf, map[string]interface{}{"retry_count": toInt(state["retry_count"]) + 1})
	r.send("task.retry", "Retrying task", nil, "info")
}

// Pause signals the run is paused.
func (r *Run) Pause(message string) {
	state := ReadState(r.sf)
	UpdateState(r.sf, map[string]interface{}{
		"paused_at":   time.Now().UnixMilli(),
		"pause_count": toInt(state["pause_count"]) + 1,
	})
	r.send("task.paused", message, nil, "info")
}

// Resume signals the run has resumed.
func (r *Run) Resume(message string) {
	state   := ReadState(r.sf)
	now     := time.Now().UnixMilli()
	updates := map[string]interface{}{}
	if pausedAt := toInt64(state["paused_at"]); pausedAt > 0 {
		updates["pause_total_ms"] = toInt64(state["pause_total_ms"]) + (now - pausedAt)
		updates["paused_at"]      = nil
	}
	if waitAt := toInt64(state["wait_at"]); waitAt > 0 {
		updates["wait_total_ms"] = toInt64(state["wait_total_ms"]) + (now - waitAt)
		updates["wait_at"]       = nil
	}
	if len(updates) > 0 {
		UpdateState(r.sf, updates)
	}
	r.send("task.resumed", message, nil, "info")
}

// Wait signals the run is waiting for an external dependency.
func (r *Run) Wait(message string) {
	state := ReadState(r.sf)
	UpdateState(r.sf, map[string]interface{}{
		"wait_at":    time.Now().UnixMilli(),
		"wait_count": toInt(state["wait_count"]) + 1,
	})
	r.send("task.waiting", message, nil, "info")
}

// Stop signals a non-terminal stop.
func (r *Run) Stop() {
	r.send("task.stopped", "Task stopped", nil, "info")
}

// Error signals a non-fatal error — the run continues.
func (r *Run) Error(message string) {
	state := ReadState(r.sf)
	UpdateState(r.sf, map[string]interface{}{
		"last_error":  message,
		"error_count": toInt(state["error_count"]) + 1,
	})
	r.send("task.error", message, nil, "error")
}

// Complete signals successful completion (terminal).
func (r *Run) Complete(message string) {
	r.send("task.completed", message, nil, "info")
	r.terminal()
}

// Fail signals a terminal failure.
func (r *Run) Fail(message string) {
	r.send("task.failed", message, nil, "error")
	r.terminal()
}

// Timeout signals a terminal timeout.
func (r *Run) Timeout(message string) {
	r.send("task.timeout", message, nil, "error")
	r.terminal()
}

// Cancel signals a terminal cancellation.
func (r *Run) Cancel(message string) {
	r.send("task.cancelled", message, nil, "info")
	r.terminal()
}

// Terminate signals a terminal force-termination.
func (r *Run) Terminate(message string) {
	r.send("task.terminated", message, nil, "error")
	r.terminal()
}

// ── Run input / output ────────────────────────────────────────────────────────

func (r *Run) InputRequired(message string)  { r.send("input.required",  message, nil, "info") }
func (r *Run) InputApproved(message string)  { r.send("input.approved",  message, nil, "info") }
func (r *Run) InputRejected(message string)  { r.send("input.rejected",  message, nil, "info") }
func (r *Run) OutputGenerated(message string) { r.send("output.generated", message, nil, "info") }
func (r *Run) OutputFailed(message string)   { r.send("output.failed",   message, nil, "error") }

// Track sends a free-form run-level event.
func (r *Run) Track(event, message string, opts ...TrackOptions) {
	meta := map[string]interface{}{}
	if len(opts) > 0 && opts[0].Meta != nil {
		meta = opts[0].Meta
	}
	r.send(event, message, meta, "info")
}

// ── Run internals ─────────────────────────────────────────────────────────────

func (r *Run) send(event, message string, extraMeta map[string]interface{}, level string) {
	if extraMeta == nil {
		extraMeta = map[string]interface{}{}
	}
	r.agent.sendPayload(event, message, r.RunId, r.label, r.sf, r.metrics, extraMeta, level)
}

func (r *Run) terminal() {
	DeleteState(r.sf)
	DeletePointer(r.agent.agent, r.label)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func genRunId() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("run_%d_%s", time.Now().UnixMilli(), hex.EncodeToString(b))
}
