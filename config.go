package notilens

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// AgentConfig holds credentials for one agent.
type AgentConfig struct {
	Token  string `json:"token"`
	Secret string `json:"secret"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".notilens_config.json")
}

func loadConfig() map[string]AgentConfig {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return map[string]AgentConfig{}
	}
	var cfg map[string]AgentConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return map[string]AgentConfig{}
	}
	return cfg
}

func saveConfig(cfg map[string]AgentConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

// SaveAgent saves credentials for an agent to ~/.notilens_config.json.
func SaveAgent(agent, token, secret string) error {
	cfg := loadConfig()
	cfg[agent] = AgentConfig{Token: token, Secret: secret}
	return saveConfig(cfg)
}

// GetAgent returns credentials for an agent.
func GetAgent(agent string) (AgentConfig, bool) {
	cfg := loadConfig()
	c, ok := cfg[agent]
	return c, ok
}

// RemoveAgent removes an agent from config.
func RemoveAgent(agent string) bool {
	cfg := loadConfig()
	if _, ok := cfg[agent]; !ok {
		return false
	}
	delete(cfg, agent)
	_ = saveConfig(cfg)
	return true
}

// ListAgents returns all configured agent names.
func ListAgents() []string {
	cfg := loadConfig()
	agents := make([]string, 0, len(cfg))
	for a := range cfg {
		agents = append(agents, a)
	}
	return agents
}
