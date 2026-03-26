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

	case "task.start":
		flags := parseFlags(rest)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.WriteState(sf, map[string]interface{}{
			"agent":       flags.agent,
			"task":        flags.taskID,
			"start_time":  time.Now().UnixMilli(),
			"retry_count": 0,
			"loop_count":  0,
		})
		sendNotify("task.started", "Task started", flags)
		fmt.Printf("▶  Started: %s | %s\n", flags.agent, flags.taskID)

	case "task.progress":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.progress", msg, flags)
		fmt.Printf("⏳ Progress: %s | %s\n", flags.agent, flags.taskID)

	case "task.loop":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		state := notilens.ReadState(sf)
		loopCount := toInt(state["loop_count"]) + 1
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "loop_count": loopCount})
		sendNotify("task.loop", msg, flags)
		fmt.Printf("🔄 Loop (%d): %s | %s\n", loopCount, flags.agent, flags.taskID)

	case "task.retry":
		flags := parseFlags(rest)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		state := notilens.ReadState(sf)
		retryCount := toInt(state["retry_count"]) + 1
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "retry_count": retryCount})
		sendNotify("task.retry", "Retrying task", flags)
		fmt.Printf("🔁 Retry: %s | %s\n", flags.agent, flags.taskID)

	case "task.stop":
		flags := parseFlags(rest)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		dur := calcDuration(sf)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": dur})
		sendNotify("task.stopped", "Task stopped", flags)
		fmt.Printf("⏹  Stopped: %s | %s (%d ms)\n", flags.agent, flags.taskID, dur)

	case "task.error":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf), "last_error": msg})
		sendNotify("task.error", msg, flags)
		fmt.Fprintf(os.Stderr, "❌ Error: %s\n", msg)

	case "task.fail":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.failed", msg, flags)
		notilens.DeleteState(sf)
		fmt.Printf("💥 Failed: %s | %s\n", flags.agent, flags.taskID)

	case "task.timeout":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.timeout", msg, flags)
		notilens.DeleteState(sf)
		fmt.Printf("⏰ Timeout: %s | %s\n", flags.agent, flags.taskID)

	case "task.cancel":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.cancelled", msg, flags)
		notilens.DeleteState(sf)
		fmt.Printf("🚫 Cancelled: %s | %s\n", flags.agent, flags.taskID)

	case "task.terminate":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.terminated", msg, flags)
		notilens.DeleteState(sf)
		fmt.Printf("⚠  Terminated: %s | %s\n", flags.agent, flags.taskID)

	case "task.complete":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify("task.completed", msg, flags)
		notilens.DeleteState(sf)
		fmt.Printf("✅ Completed: %s | %s\n", flags.agent, flags.taskID)

	case "metric":
		pos, rest2 := positionalArgs(rest)
		flags := parseFlags(rest2)
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
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
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
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
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sendNotify("output.generated", msg, flags)

	case "output.fail":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sendNotify("output.failed", msg, flags)

	case "input.required":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sendNotify("input.required", msg, flags)

	case "input.approve":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sendNotify("input.approved", msg, flags)

	case "input.reject":
		pos, rest2 := positionalArgs(rest)
		msg := ""
		if len(pos) > 0 {
			msg = pos[0]
		}
		flags := parseFlags(rest2)
		sendNotify("input.rejected", msg, flags)

	case "emit":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: notilens emit <event> <message> --agent <agent>")
			os.Exit(1)
		}
		event := rest[0]
		msg := rest[1]
		flags := parseFlags(rest[2:])
		sf := notilens.GetStateFile(flags.agent, flags.taskID)
		notilens.UpdateState(sf, map[string]interface{}{"duration_ms": calcDuration(sf)})
		sendNotify(event, msg, flags)
		fmt.Printf("📡 Event emitted: %s\n", event)

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
	taskID       string
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
				f.taskID = args[i+1]
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
	if f.taskID == "" {
		f.taskID = fmt.Sprintf("task_%d", time.Now().UnixMilli())
	}
	return f
}

// ── Core send ─────────────────────────────────────────────────────────────────

var successEvents = map[string]bool{
	"task.completed":   true,
	"output.generated": true,
	"input.approved":   true,
}
var urgentEvents = map[string]bool{
	"task.failed":    true,
	"task.timeout":   true,
	"task.error":     true,
	"task.terminated": true,
	"output.failed":  true,
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

func sendNotify(event, message string, f flags) {
	conf, ok := notilens.GetAgent(f.agent)
	if !ok || conf.Token == "" || conf.Secret == "" {
		fmt.Fprintf(os.Stderr,
			"❌ Agent '%s' not configured. Run: notilens init --agent %s --token TOKEN --secret SECRET\n",
			f.agent, f.agent)
		os.Exit(1)
	}

	sf := notilens.GetStateFile(f.agent, f.taskID)
	state := notilens.ReadState(sf)

	meta := map[string]interface{}{"agent": f.agent}
	if d := toInt64(state["duration_ms"]); d > 0 {
		meta["duration_ms"] = d
	}
	if r := toInt64(state["retry_count"]); r > 0 {
		meta["retry_count"] = r
	}
	if l := toInt64(state["loop_count"]); l > 0 {
		meta["loop_count"] = l
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

	title := f.agent + " | " + f.taskID + " | " + event
	if f.taskID == "" {
		title = f.agent + " | " + event
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
		"task_id":       f.taskID,
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

func calcDuration(stateFile string) int64 {
	s := notilens.ReadState(stateFile)
	start := toInt64(s["start_time"])
	if start == 0 {
		return 0
	}
	return time.Now().UnixMilli() - start
}

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

// ── Usage ─────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Print(`Usage:
  notilens init --agent <name> --token <token> --secret <secret>
  notilens agents
  notilens remove-agent <agent>

Task Lifecycle:
  notilens task.start     --agent <agent> [--task <id>]
  notilens task.progress  "msg" --agent <agent> [--task <id>]
  notilens task.loop      "msg" --agent <agent> [--task <id>]
  notilens task.retry           --agent <agent> [--task <id>]
  notilens task.stop            --agent <agent> [--task <id>]
  notilens task.error     "msg" --agent <agent> [--task <id>]
  notilens task.fail      "msg" --agent <agent> [--task <id>]
  notilens task.timeout   "msg" --agent <agent> [--task <id>]
  notilens task.cancel    "msg" --agent <agent> [--task <id>]
  notilens task.terminate "msg" --agent <agent> [--task <id>]
  notilens task.complete  "msg" --agent <agent> [--task <id>]

Output / Input:
  notilens output.generate "msg" --agent <agent> [--task <id>]
  notilens output.fail     "msg" --agent <agent> [--task <id>]
  notilens input.required  "msg" --agent <agent> [--task <id>]
  notilens input.approve   "msg" --agent <agent> [--task <id>]
  notilens input.reject    "msg" --agent <agent> [--task <id>]

Metrics:
  notilens metric       tokens=512 cost=0.003 --agent <agent> --task <id>
  notilens metric.reset tokens               --agent <agent> --task <id>
  notilens metric.reset                      --agent <agent> --task <id>

Generic:
  notilens emit <event> "msg" --agent <agent>

Options:
  --agent <name>
  --task <id>
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
