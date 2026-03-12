package config

import (
	"os"
	"strconv"
)

// Config holds all configuration for the observability platform.
type Config struct {
	ServerPort    int
	DatabaseURL   string
	AlertRulesDir string
	WebDir        string
	AgentInterval int // seconds between metric pushes
	ServerURL     string
	LogLevel      string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		ServerPort:    getEnvInt("OBS_SERVER_PORT", 9090),
		DatabaseURL:   getEnv("OBS_DATABASE_URL", "postgres://obsuser:obspass@localhost:5432/observability?sslmode=disable"),
		AlertRulesDir: getEnv("OBS_ALERT_RULES", "configs/alerts.yaml"),
		WebDir:        getEnv("OBS_WEB_DIR", "web"),
		AgentInterval: getEnvInt("OBS_AGENT_INTERVAL", 15),
		ServerURL:     getEnv("OBS_SERVER_URL", "http://localhost:9090"),
		LogLevel:      getEnv("OBS_LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}
