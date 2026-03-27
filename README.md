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

run := nl.Task("report")
run.Start()
run.Progress("Processing...")
run.Complete("Done!")
```

## Credentials

Resolved in order:
1. `Options{Token: "...", Secret: "..."}` passed to `Init()`
2. `NOTILENS_TOKEN` / `NOTILENS_SECRET` env vars
3. Saved CLI config (`notilens init --agent ...`)

```go
nl, err := notilens.Init("my-agent", notilens.Options{
    Token:    "your-token",
    Secret:   "your-secret",
    StateTtl: 86400, // optional — orphaned state TTL in seconds (default: 86400)
})
```

## SDK Reference

### Task Lifecycle

`nl.Task(label)` creates a `Run` — an isolated execution context. Multiple concurrent runs of the same label never conflict.

```go
run := nl.Task("email")  // create a run for the "email" task
run.Queue()              // optional — pre-start signal
run.Start()              // begin the run

run.Progress("Fetching data...")
run.Loop("Processing item 42")
run.Retry()
run.Pause("Waiting for rate limit")
run.Resume("Resuming work")
run.Wait("Waiting for tool response")
run.Stop()
run.Error("Quota exceeded") // non-fatal, run continues

// Terminal — pick one
run.Complete("All done!")
run.Fail("Unrecoverable error")
run.Timeout("Timed out after 5m")
run.Cancel("Cancelled by user")
run.Terminate("Force-killed")
```

### Output & Input Events

```go
run.OutputGenerated("Report ready")
run.OutputFailed("Rendering failed")

run.InputRequired("Approve deployment?")
run.InputApproved("Approved")
run.InputRejected("Rejected")
```

### Metrics

Numeric values accumulate; strings are replaced.

```go
run.Metric("tokens", 512)
run.Metric("tokens", 128)   // now 640
run.Metric("cost", 0.003)

run.ResetMetrics("tokens")  // reset one key
run.ResetMetrics()          // reset all
```

Metrics are automatically included in every notification's metadata.

### Automatic Timing

NotiLens automatically tracks task timing. These fields are included in every notification's `meta` payload when non-zero:

| Field | Description |
|-------|-------------|
| `total_duration_ms` | Wall-clock time since `start` |
| `queue_ms` | Time between `queue` and `start` |
| `pause_ms` | Cumulative time spent paused |
| `wait_ms` | Cumulative time spent waiting |
| `active_ms` | Active time (`total − pause − wait`) |

### Generic Events

```go
nl.Track("custom.event", "Something happened")
nl.Track("custom.event", "With meta", notilens.TrackOptions{
    Meta: map[string]interface{}{"key": "value"},
})

run.Track("custom.event", "Run-level event")
```

### Full Example

```go
import (
    "log"
    notilens "github.com/notilens/sdk-go"
)

nl, err := notilens.Init("summarizer", notilens.Options{
    Token:  "TOKEN",
    Secret: "SECRET",
})
if err != nil {
    log.Fatal(err)
}

run := nl.Task("report")
run.Start()

result, err := llm.Complete(prompt)
if err != nil {
    run.Fail(err.Error())
    return
}

run.Metric("tokens", result.Usage.TotalTokens)
run.OutputGenerated("Summary ready")
run.Complete("All done!")
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

`--task` is a semantic label (e.g. `email`, `report`). Each `task.start` creates an isolated run internally — concurrent executions of the same label never conflict.

```bash
# Task lifecycle
notilens queue                      --agent my-agent --task email
notilens start                      --agent my-agent --task email
notilens progress  "Fetching data"  --agent my-agent --task email
notilens loop      "Item 5/100"     --agent my-agent --task email
notilens retry                      --agent my-agent --task email
notilens pause     "Rate limited"   --agent my-agent --task email
notilens resume    "Resuming"       --agent my-agent --task email
notilens wait      "Awaiting tool"  --agent my-agent --task email
notilens stop                       --agent my-agent --task email
notilens error     "Quota hit"      --agent my-agent --task email
notilens fail      "Fatal error"    --agent my-agent --task email
notilens timeout   "Timed out"      --agent my-agent --task email
notilens cancel    "Cancelled"      --agent my-agent --task email
notilens terminate "Force stop"     --agent my-agent --task email
notilens complete  "Done!"          --agent my-agent --task email

# Output / Input
notilens output.generate "Report ready"  --agent my-agent --task email
notilens output.fail     "Render failed" --agent my-agent --task email
notilens input.required  "Approve?"      --agent my-agent --task email
notilens input.approve   "Approved"      --agent my-agent --task email
notilens input.reject    "Rejected"      --agent my-agent --task email

# Metrics (accumulated per run)
notilens metric       tokens=512 cost=0.003 --agent my-agent --task email
notilens metric.reset tokens               --agent my-agent --task email
notilens metric.reset                      --agent my-agent --task email

# Generic
notilens track my.event "Something happened" --agent my-agent

# Version
notilens version
```

`task.start` prints the internal `run_id` to stdout.

### Options

| Flag | Description |
|---|---|
| `--agent <name>` | Agent name (required) |
| `--task <label>` | Task label (e.g. `email`, `report`) |
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
