package notilens

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func osUser() string {
	u := os.Getenv("USER")
	if u == "" {
		u = os.Getenv("USERNAME")
	}
	if u == "" {
		u = "default"
	}
	return u
}

// GetStateFile returns the state file path for a given agent + runId.
func GetStateFile(agent, runId string) string {
	user  := osUser()
	agent  = strings.ReplaceAll(agent, string(filepath.Separator), "_")
	runId  = strings.ReplaceAll(runId, string(filepath.Separator), "_")
	return filepath.Join(os.TempDir(), fmt.Sprintf("notilens_%s_%s_%s.json", user, agent, runId))
}

// GetPointerFile returns the pointer file path for a given agent + label.
func GetPointerFile(agent, label string) string {
	user      := osUser()
	safeLabel := strings.NewReplacer("/", "_", "\\", "_").Replace(label)
	return filepath.Join(os.TempDir(), fmt.Sprintf("notilens_%s_%s_%s.ptr", user, agent, safeLabel))
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

// WriteState writes a state map to a file atomically.
func WriteState(file string, s map[string]interface{}) {
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, file)
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
func DeleteState(file string) { _ = os.Remove(file) }

// ReadPointer reads the run_id from a pointer file.
func ReadPointer(agent, label string) string {
	data, err := os.ReadFile(GetPointerFile(agent, label))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// WritePointer writes a run_id to a pointer file.
func WritePointer(agent, label, runId string) {
	_ = os.WriteFile(GetPointerFile(agent, label), []byte(runId), 0600)
}

// DeletePointer removes a pointer file.
func DeletePointer(agent, label string) { _ = os.Remove(GetPointerFile(agent, label)) }

// CleanupStaleState removes state and pointer files older than stateTtlSeconds.
func CleanupStaleState(agent string, stateTtlSeconds int) {
	user   := osUser()
	tmp    := os.TempDir()
	cutoff := time.Now().Add(-time.Duration(stateTtlSeconds) * time.Second)
	prefix := fmt.Sprintf("notilens_%s_%s_", user, agent)

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if !strings.HasSuffix(name, ".json") && !strings.HasSuffix(name, ".ptr") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(tmp, name))
		}
	}
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
