# NotiLens Go SDK

Go SDK and CLI for [NotiLens](https://notilens.com) — send task lifecycle notifications from AI agents, background jobs, and any Go application.

## Installation

```bash
go get github.com/notilens/sdk-go
```

## Quick Start

```go
import notilens "github.com/notilens/sdk-go"

nl, err := notilens.Init("my-agent")
if err != nil {
    log.Fatal(err)
}

taskID := nl.TaskStart()
nl.TaskProgress("Processing...", taskID)
nl.TaskComplete("Done!", taskID)
```

## Credentials

Resolved in order:
1. `Options{Token: "...", Secret: "..."}` passed to `Init()`
2. `NOTILENS_TOKEN` / `NOTILENS_SECRET` env vars
3. Saved CLI config (`notilens init --agent ...`)

```go
nl, err := notilens.Init("my-agent", notilens.Options{
    Token:  "your-token",
    Secret: "your-secret",
})
```

## SDK Reference

### Task Lifecycle

```go
taskID := nl.TaskStart()                         // auto-generates ID
taskID  = nl.TaskStart("my-task-123")            // custom ID

nl.TaskProgress("Fetching data...", taskID)
nl.TaskLoop("Processing item 42", taskID)
nl.TaskRetry(taskID)
nl.TaskStop(taskID)
nl.TaskError("Quota exceeded", taskID)           // non-fatal
nl.TaskComplete("All done!", taskID)             // terminal
nl.TaskFail("Unrecoverable error", taskID)       // terminal
nl.TaskTimeout("Timed out after 5m", taskID)     // terminal
nl.TaskCancel("Cancelled by user", taskID)       // terminal
nl.TaskTerminate("Force-killed", taskID)         // terminal
```

### Output & Input Events

```go
nl.OutputGenerated("Report ready", taskID)
nl.OutputFailed("Rendering failed", taskID)

nl.InputRequired("Approve deployment?", taskID)
nl.InputApproved("Approved", taskID)
nl.InputRejected("Rejected", taskID)
```

### Metrics

Numeric values accumulate; strings are replaced.

```go
nl.Metric("tokens", 512)
nl.Metric("tokens", 128)   // now 640

nl.ResetMetrics("tokens")  // reset one key
nl.ResetMetrics()          // reset all
```

Metrics are automatically included in every notification's metadata.

### Generic Events

```go
nl.Emit("custom.event", "Something happened")
nl.Emit("custom.event", "With meta", notilens.EmitOptions{
    Meta: map[string]interface{}{"key": "value"},
})
```

## CLI

### Install

```bash
go install github.com/notilens/sdk-go/cmd/notilens@latest
```

### Configure

```bash
notilens init --agent my-agent --token TOKEN --secret SECRET
notilens agents
notilens remove-agent my-agent
```

### Commands

```bash
# Task lifecycle
notilens task.start     --agent my-agent --task job-123
notilens task.progress  "Fetching data" --agent my-agent --task job-123
notilens task.loop      "Item 5/100"    --agent my-agent --task job-123
notilens task.retry                     --agent my-agent --task job-123
notilens task.stop                      --agent my-agent --task job-123
notilens task.error     "Quota hit"     --agent my-agent --task job-123
notilens task.fail      "Fatal error"   --agent my-agent --task job-123
notilens task.timeout   "Timed out"     --agent my-agent --task job-123
notilens task.cancel    "Cancelled"     --agent my-agent --task job-123
notilens task.terminate "Force stop"    --agent my-agent --task job-123
notilens task.complete  "Done!"         --agent my-agent --task job-123

# Output / Input
notilens output.generate "Report ready"       --agent my-agent --task job-123
notilens output.fail     "Render failed"      --agent my-agent --task job-123
notilens input.required  "Approve?"           --agent my-agent --task job-123
notilens input.approve   "Approved"           --agent my-agent --task job-123
notilens input.reject    "Rejected"           --agent my-agent --task job-123

# Metrics (accumulated per task)
notilens metric       tokens=512 cost=0.003   --agent my-agent --task job-123
notilens metric.reset tokens                  --agent my-agent --task job-123
notilens metric.reset                         --agent my-agent --task job-123

# Generic
notilens emit my.event "Something happened"   --agent my-agent

# Version
notilens version
```

### Options

| Flag | Description |
|---|---|
| `--agent <name>` | Agent name (required) |
| `--task <id>` | Task ID (auto-generated if omitted) |
| `--type success\|warning\|urgent\|info` | Override notification type |
| `--meta key=value` | Extra metadata (repeatable) |
| `--image_url <url>` | Attach image |
| `--open_url <url>` | Action URL |
| `--download_url <url>` | Download URL |
| `--tags "tag1,tag2"` | Tags |
| `--is_actionable true\|false` | Override actionable flag |

## Requirements

- Go 1.21+
- No external dependencies
