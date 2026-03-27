package notilens

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	webhookURL = "https://hook.notilens.com/webhook/%s/send"
	Version    = "0.4.0"
)

var successEvents = map[string]bool{
	"task.completed":   true,
	"output.generated": true,
	"input.approved":   true,
}

var urgentEvents = map[string]bool{
	"task.failed":   true,
	"task.timeout":  true,
	"task.error":    true,
	"task.terminated": true,
	"output.failed": true,
}

var warningEvents = map[string]bool{
	"task.retry":     true,
	"task.cancelled": true,
	"task.paused":    true,
	"task.waiting":   true,
	"input.required": true,
	"input.rejected": true,
}

var actionableEvents = map[string]bool{
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

func sendHTTP(token, secret string, payload map[string]interface{}) error {
	url  := fmt.Sprintf(webhookURL, token)
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
	req.Header.Set("User-Agent", "NotiLens-SDK/"+Version)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
