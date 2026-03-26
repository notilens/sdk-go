package notilens

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetStateFile returns the path to the state file for an agent+task.
func GetStateFile(agent, taskID string) string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	agent  = strings.ReplaceAll(agent,  string(filepath.Separator), "_")
	taskID = strings.ReplaceAll(taskID, string(filepath.Separator), "_")
	return filepath.Join(os.TempDir(), "notilens_"+user+"_"+agent+"_"+taskID+".json")
}

// ReadState reads a state file into a map.
func ReadState(file string) map[string]interface{} {
	data, err := os.ReadFile(file)
	if err != nil {
		return map[string]interface{}{}
	}
	var s map[string]interface{}
	if err := json.Unmarshal(data, &s); err != nil || s == nil {
		return map[string]interface{}{}
	}
	return s
}

// WriteState writes a state map to a file.
func WriteState(file string, s map[string]interface{}) {
	data, _ := json.MarshalIndent(s, "", "  ")
	_ = os.WriteFile(file, data, 0600)
}

// UpdateState merges updates into an existing state file.
func UpdateState(file string, updates map[string]interface{}) {
	s := ReadState(file)
	for k, v := range updates {
		s[k] = v
	}
	WriteState(file, s)
}

// DeleteState removes a state file.
func DeleteState(file string) {
	_ = os.Remove(file)
}

// calcDuration returns elapsed ms since task start, or 0 if not started.
func calcDuration(stateFile string) int64 {
	s := ReadState(stateFile)
	start := toInt64(s["start_time"])
	if start == 0 {
		return 0
	}
	return time.Now().UnixMilli() - start
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
