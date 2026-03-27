package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	notilens "github.com/notilens/sdk-go"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	command := os.Args[1]
	rest := os.Args[2:]

	switch command {

	case "init":
		runInit(rest)

	case "agents":
		agents := notilens.ListAgents()
		if len(agents) == 0 {
			fmt.Println("No agents configured.")
		} else {
			for _, a := range agents {
				fmt.Println(" ", a)
			}
		}

	case "remove-agent":
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: notilens remove-agent <agent>")
			os.Exit(1)
		}
		if notilens.RemoveAgent(rest[0]) {
			fmt.Printf("✔ Agent '%s' removed\n", rest[0])
		} else {
			fmt.Fprintf(os.Stderr, "Agent '%s' not found\n", rest[0])
		}

	case "queue":
		flags := parseFlags(rest)
		runId := genRunId()
		sf := notilens.GetStateFile(flags.agent, runId)
		notilens.WriteState(sf, map[string]interface{}{
			"agent":          flags.agent,
			"task":           flags.taskLabel,
			"run_id":         runId,
			"queued_at":      time.Now().UnixMilli(),
			"retry_count":    0,
			"loop_count":     0,
			"error_count":    0,
			"pause_count":    0,
			"wait_count":     0,
			"pause_total_ms": 0,
			"wait_total_ms":  0,
		})
		notilens.WritePointer(flags.agent, flags.taskLabel, runId)
		sendNotify("task.queued", "Task queued", flags, runId)
		fmt.Println(runId)

	case "start":
		flags := parseFlags(rest)
		// Reuse run_id from a prior task.queue if available
		runId := notilens.ReadPointer(flags.agent, flags.taskLabel)
		if runId == "" {
			runId = genRunId()
		}
		sf := notilens.GetStateFile(flags.agent, runId)
		existing := notilens.ReadState(sf)
		if len(existing) > 0 {
			notilens.UpdateState(sf, map[string]interface{}{"start_time": time.Now().UnixMilli()})
		} else {
			notilens.WriteState(sf, map[string]interface{}{
				"agent":          flags.agent,
				"task":           flags.taskLabel,
				"run_id":         runId,
				"start_time":     time.Now().UnixMilli(),
				"retry_count":    0,
				"loop_count":     0,
				"error_count":    0,
				"pause_count":    0,
				"wait_count":     0,
				"pause_total_ms": 0,
				"wait_total_ms":  0,
			})
		}
		notilens.WritePointer(flags.agent, flags.taskLabel, runId)
		sendNotify("task.started", "Task started", flags, runId)
		fmt.Println(runId)

	case "progress":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.progress", msg, flags, runId)

	case "loop":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		loopCount := toInt(state["loop_count"]) + 1
		notilens.UpdateState(sf, map[string]interface{}{"loop_count": loopCount})
		sendNotify("task.loop", msg, flags, runId)

	case "retry":
		flags := parseFlags(rest)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		retryCount := toInt(state["retry_count"]) + 1
		notilens.UpdateState(sf, map[string]interface{}{"retry_count": retryCount})
		sendNotify("task.retry", "Retrying task", flags, runId)

	case "stop":
		flags := parseFlags(rest)
		runId := resolveRunId(flags)
		sendNotify("task.stopped", "Task stopped", flags, runId)

	case "pause":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		notilens.UpdateState(sf, map[string]interface{}{
			"paused_at":   time.Now().UnixMilli(),
			"pause_count": toInt(state["pause_count"]) + 1,
		})
		sendNotify("task.paused", msg, flags, runId)

	case "resume":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		now := time.Now().UnixMilli()
		updates := map[string]interface{}{}
		if pausedAt := toInt64(state["paused_at"]); pausedAt > 0 {
			updates["pause_total_ms"] = toInt64(state["pause_total_ms"]) + (now - pausedAt)
			updates["paused_at"] = nil
		}
		if waitAt := toInt64(state["wait_at"]); waitAt > 0 {
			updates["wait_total_ms"] = toInt64(state["wait_total_ms"]) + (now - waitAt)
			updates["wait_at"] = nil
		}
		if len(updates) > 0 {
			notilens.UpdateState(sf, updates)
		}
		sendNotify("task.resumed", msg, flags, runId)

	case "wait":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		notilens.UpdateState(sf, map[string]interface{}{
			"wait_at":    time.Now().UnixMilli(),
			"wait_count": toInt(state["wait_count"]) + 1,
		})
		sendNotify("task.waiting", msg, flags, runId)

	case "error":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		notilens.UpdateState(sf, map[string]interface{}{
			"last_error":  msg,
			"error_count": toInt(state["error_count"]) + 1,
		})
		sendNotify("task.error", msg, flags, runId)

	case "fail":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.failed", msg, flags, runId)
		notilens.DeleteState(notilens.GetStateFile(flags.agent, runId))
		notilens.DeletePointer(flags.agent, flags.taskLabel)

	case "timeout":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.timeout", msg, flags, runId)
		notilens.DeleteState(notilens.GetStateFile(flags.agent, runId))
		notilens.DeletePointer(flags.agent, flags.taskLabel)

	case "cancel":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.cancelled", msg, flags, runId)
		notilens.DeleteState(notilens.GetStateFile(flags.agent, runId))
		notilens.DeletePointer(flags.agent, flags.taskLabel)

	case "terminate":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.terminated", msg, flags, runId)
		notilens.DeleteState(notilens.GetStateFile(flags.agent, runId))
		notilens.DeletePointer(flags.agent, flags.taskLabel)

	case "complete":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("task.completed", msg, flags, runId)
		notilens.DeleteState(notilens.GetStateFile(flags.agent, runId))
		notilens.DeletePointer(flags.agent, flags.taskLabel)

	case "metric":
		pos, rest2 := positionalArgs(rest)
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		state := notilens.ReadState(sf)
		metrics := map[string]interface{}{}
		if m, ok := state["metrics"]; ok {
			if mm, ok := m.(map[string]interface{}); ok {
				metrics = mm
			}
		}
		for _, kv := range pos {
			eq := strings.Index(kv, "=")
			if eq < 0 {
				continue
			}
			k := kv[:eq]
			v := kv[eq+1:]
			if fv, err := strconv.ParseFloat(v, 64); err == nil {
				if existing, ok := metrics[k]; ok {
					if fe, ok := toFloat64(existing); ok {
						metrics[k] = fe + fv
						continue
					}
				}
				metrics[k] = fv
			} else {
				metrics[k] = v
			}
		}
		notilens.UpdateState(sf, map[string]interface{}{"metrics": metrics})
		parts := []string{}
		for k, v := range metrics {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		fmt.Printf("📊 Metrics: %s\n", strings.Join(parts, ", "))

	case "metric.reset":
		pos, rest2 := positionalArgs(rest)
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sf := notilens.GetStateFile(flags.agent, runId)
		if len(pos) > 0 {
			state := notilens.ReadState(sf)
			metrics := map[string]interface{}{}
			if m, ok := state["metrics"]; ok {
				if mm, ok := m.(map[string]interface{}); ok {
					metrics = mm
				}
			}
			delete(metrics, pos[0])
			notilens.UpdateState(sf, map[string]interface{}{"metrics": metrics})
			fmt.Printf("📊 Metric '%s' reset\n", pos[0])
		} else {
			notilens.UpdateState(sf, map[string]interface{}{"metrics": map[string]interface{}{}})
			fmt.Println("📊 All metrics reset")
		}

	case "output.generate":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("output.generated", msg, flags, runId)

	case "output.fail":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("output.failed", msg, flags, runId)

	case "input.required":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("input.required", msg, flags, runId)

	case "input.approve":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("input.approved", msg, flags, runId)

	case "input.reject":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 { msg = pos[0] }
		flags := parseFlags(rest2)
		runId := resolveRunId(flags)
		sendNotify("input.rejected", msg, flags, runId)

	case "track":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: notilens track <event> <message> --agent <agent>")
			os.Exit(1)
		}
		event := rest[0]
		msg := rest[1]
		flags := parseFlags(rest[2:])
		// track is agent-level; use pointer if available but don't error if absent
		runId := notilens.ReadPointer(flags.agent, flags.taskLabel)
		sendNotify(event, msg, flags, runId)
		fmt.Printf("📡 Tracked: %s\n", event)

	case "version":
		fmt.Printf("NotiLens v%s\n", notilens.Version)

	default:
		printUsage()
		os.Exit(1)
	}
}

// ── Flag parsing ──────────────────────────────────────────────────────────────

type flags struct {
	agent        string
	taskLabel    string
	typ          string
	meta         map[string]string
	imageURL     string
	openURL      string
	downloadURL  string
	tags         string
	isActionable string
}

// positionalArgs returns args before the first --flag and the remainder.
func positionalArgs(args []string) ([]string, []string) {
	var pos []string
	for i, a := range args {
		if strings.HasPrefix(a, "--") {
			return pos, args[i:]
		}
		pos = append(pos, a)
	}
	return pos, nil
}

func parseFlags(args []string) flags {
	f := flags{meta: map[string]string{}}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				f.agent = args[i+1]
				i++
			}
		case "--task":
			if i+1 < len(args) {
				f.taskLabel = args[i+1]
				i++
			}
		case "--type":
			if i+1 < len(args) {
				f.typ = args[i+1]
				i++
			}
		case "--image_url":
			if i+1 < len(args) {
				f.imageURL = args[i+1]
				i++
			}
		case "--open_url":
			if i+1 < len(args) {
				f.openURL = args[i+1]
				i++
			}
		case "--download_url":
			if i+1 < len(args) {
				f.downloadURL = args[i+1]
				i++
			}
		case "--tags":
			if i+1 < len(args) {
				f.tags = args[i+1]
				i++
			}
		case "--is_actionable":
			if i+1 < len(args) {
				f.isActionable = args[i+1]
				i++
			}
		case "--meta":
			if i+1 < len(args) {
				kv := args[i+1]
				i++
				if eq := strings.Index(kv, "="); eq >= 0 {
					f.meta[kv[:eq]] = kv[eq+1:]
				}
			}
		}
	}
	if f.agent == "" {
		fmt.Fprintln(os.Stderr, "❌ --agent is required")
		os.Exit(1)
	}
	return f
}

// resolveRunId reads the pointer file for the current task label.
// Exits with an error if no pointer is found (start was not called).
func resolveRunId(f flags) string {
	if f.taskLabel == "" {
		fmt.Fprintln(os.Stderr, "❌ --task is required")
		os.Exit(1)
	}
	runId := notilens.ReadPointer(f.agent, f.taskLabel)
	if runId == "" {
		fmt.Fprintf(os.Stderr,
			"❌ No active run for task '%s' on agent '%s'. Run start first.\n",
			f.taskLabel, f.agent)
		os.Exit(1)
	}
	return runId
}

// ── Core send ─────────────────────────────────────────────────────────────────

var successEvents = map[string]bool{
	"task.completed":   true,
	"output.generated": true,
	"input.approved":   true,
}
var urgentEvents = map[string]bool{
	"task.failed":     true,
	"task.timeout":    true,
	"task.error":      true,
	"task.terminated": true,
	"output.failed":   true,
}
var warningEvents = map[string]bool{
	"task.retry":     true,
	"task.cancelled": true,
	"input.required": true,
	"input.rejected": true,
}
var actionableEventsCLI = map[string]bool{
	"task.error":     true,
	"task.failed":    true,
	"task.timeout":   true,
	"task.retry":     true,
	"task.loop":      true,
	"output.failed":  true,
	"input.required": true,
	"input.rejected": true,
}

func getEventType(event string) string {
	if successEvents[event] {
		return "success"
	}
	if urgentEvents[event] {
		return "urgent"
	}
	if warningEvents[event] {
		return "warning"
	}
	return "info"
}

func sendNotify(event, message string, f flags, runId string) {
	conf, ok := notilens.GetAgent(f.agent)
	if !ok || conf.Token == "" || conf.Secret == "" {
		fmt.Fprintf(os.Stderr,
			"❌ Agent '%s' not configured. Run: notilens init --agent %s --token TOKEN --secret SECRET\n",
			f.agent, f.agent)
		os.Exit(1)
	}

	sf    := notilens.GetStateFile(f.agent, runId)
	state := notilens.ReadState(sf)

	meta := map[string]interface{}{"agent": f.agent}
	if runId != "" {
		meta["run_id"] = runId
	}
	if f.taskLabel != "" {
		meta["task"] = f.taskLabel
		now        := time.Now().UnixMilli()
		startTime  := toInt64(state["start_time"])
		queuedAt   := toInt64(state["queued_at"])
		pauseTotal := toInt64(state["pause_total_ms"])
		waitTotal  := toInt64(state["wait_total_ms"])
		if v := toInt64(state["paused_at"]); v > 0 { pauseTotal += now - v }
		if v := toInt64(state["wait_at"]);   v > 0 { waitTotal  += now - v }
		totalMs  := int64(0)
		if startTime > 0 { totalMs = now - startTime }
		queueMs  := int64(0)
		if startTime > 0 && queuedAt > 0 { queueMs = startTime - queuedAt }
		activeMs := totalMs - pauseTotal - waitTotal
		if activeMs < 0 { activeMs = 0 }

		if totalMs   > 0 { meta["total_duration_ms"] = totalMs   }
		if queueMs   > 0 { meta["queue_ms"]          = queueMs   }
		if pauseTotal > 0 { meta["pause_ms"]         = pauseTotal }
		if waitTotal  > 0 { meta["wait_ms"]          = waitTotal  }
		if activeMs  > 0 { meta["active_ms"]         = activeMs  }
		if r := toInt64(state["retry_count"]); r > 0 { meta["retry_count"] = r }
		if l := toInt64(state["loop_count"]);  l > 0 { meta["loop_count"]  = l }
		if e := toInt64(state["error_count"]); e > 0 { meta["error_count"] = e }
		if p := toInt64(state["pause_count"]); p > 0 { meta["pause_count"] = p }
		if w := toInt64(state["wait_count"]);  w > 0 { meta["wait_count"]  = w }
	}
	if m, ok := state["metrics"]; ok {
		if mm, ok := m.(map[string]interface{}); ok {
			for k, v := range mm {
				meta[k] = v
			}
		}
	}
	for k, v := range f.meta {
		meta[k] = v
	}

	title := f.agent + " | " + event
	if f.taskLabel != "" {
		title = f.agent + " | " + f.taskLabel + " | " + event
	}

	evType := getEventType(event)
	if f.typ != "" {
		switch f.typ {
		case "info", "success", "warning", "urgent":
			evType = f.typ
		}
	}

	isActionable := actionableEventsCLI[event]
	if f.isActionable != "" {
		isActionable = strings.ToLower(f.isActionable) == "true"
	}

	payload := map[string]interface{}{
		"event":         event,
		"title":         title,
		"message":       message,
		"type":          evType,
		"agent":         f.agent,
		"task_id":       f.taskLabel,
		"is_actionable": isActionable,
		"image_url":     f.imageURL,
		"open_url":      f.openURL,
		"download_url":  f.downloadURL,
		"tags":          f.tags,
		"ts":            float64(time.Now().UnixMilli()) / 1000.0,
		"meta":          meta,
	}

	_ = sendHTTP(conf.Token, conf.Secret, payload)
	time.Sleep(300 * time.Millisecond)
}

func sendHTTP(token, secret string, payload map[string]interface{}) error {
	url := fmt.Sprintf("https://hook.notilens.com/webhook/%s/send", token)
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NOTILENS-KEY", secret)
	req.Header.Set("User-Agent", "NotiLens-SDK/"+notilens.Version)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func toFloat64(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func toInt(v interface{}) int {
	f, _ := toFloat64(v)
	return int(f)
}

func toInt64(v interface{}) int64 {
	f, _ := toFloat64(v)
	return int64(f)
}

func genRunId() string {
	return fmt.Sprintf("run_%d", time.Now().UnixMilli())
}

// ── Usage ─────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Print(`Usage:
  notilens init --agent <name> --token <token> --secret <secret>
  notilens agents
  notilens remove-agent <agent>

Task Lifecycle:
  notilens queue           --agent <agent> --task <label>
  notilens start           --agent <agent> --task <label>
  notilens progress  "msg" --agent <agent> --task <label>
  notilens loop      "msg" --agent <agent> --task <label>
  notilens retry           --agent <agent> --task <label>
  notilens stop            --agent <agent> --task <label>
  notilens pause     "msg" --agent <agent> --task <label>
  notilens resume    "msg" --agent <agent> --task <label>
  notilens wait      "msg" --agent <agent> --task <label>
  notilens error     "msg" --agent <agent> --task <label>
  notilens fail      "msg" --agent <agent> --task <label>
  notilens timeout   "msg" --agent <agent> --task <label>
  notilens cancel    "msg" --agent <agent> --task <label>
  notilens terminate "msg" --agent <agent> --task <label>
  notilens complete  "msg" --agent <agent> --task <label>

Output / Input:
  notilens output.generate "msg" --agent <agent> --task <label>
  notilens output.fail     "msg" --agent <agent> --task <label>
  notilens input.required  "msg" --agent <agent> --task <label>
  notilens input.approve   "msg" --agent <agent> --task <label>
  notilens input.reject    "msg" --agent <agent> --task <label>

Metrics:
  notilens metric       tokens=512 cost=0.003 --agent <agent> --task <label>
  notilens metric.reset tokens               --agent <agent> --task <label>
  notilens metric.reset                      --agent <agent> --task <label>

Generic:
  notilens track <event> "msg" --agent <agent>

Options:
  --agent <name>
  --task <label>
  --type success|warning|urgent|info
  --meta key=value   (repeatable)
  --image_url <url>
  --open_url <url>
  --download_url <url>
  --tags "tag1,tag2"
  --is_actionable true|false

Other:
  notilens version
`)
}

// ── init command ──────────────────────────────────────────────────────────────

func runInit(args []string) {
	var agent, token, secret string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 < len(args) {
				agent = args[i+1]
				i++
			}
		case "--token":
			if i+1 < len(args) {
				token = args[i+1]
				i++
			}
		case "--secret":
			if i+1 < len(args) {
				secret = args[i+1]
				i++
			}
		}
	}
	if agent == "" || token == "" || secret == "" {
		fmt.Fprintln(os.Stderr, "Usage: notilens init --agent <name> --token <token> --secret <secret>")
		os.Exit(1)
	}
	if err := notilens.SaveAgent(agent, token, secret); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving agent: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✔ Agent '%s' saved\n", agent)
}
