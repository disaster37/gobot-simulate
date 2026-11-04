package simulate

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var simulateEnvKeys = []string{
	"SIMULATE_ADDR",
	"SIMULATE_PORT",
	"SIMULATE_UI",
	"SIMULATE_CORS",
	"SIMULATE_MAX_CLIENTS",
	"SIMULATE_EVENT_BUFFER",
	"SIMULATE_LATENCY",
	"SIMULATE_AUTH_TOKEN",
}

func clearSimulateEnv(t *testing.T) {
	t.Helper()
	for _, key := range simulateEnvKeys {
		old, ok := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		if ok {
			key, old := key, old
			t.Cleanup(func() { _ = os.Setenv(key, old) })
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "127.0.0.1", cfg.Addr)
	assert.Equal(t, 8080, cfg.Port)
	assert.True(t, cfg.UIEnabled)
	assert.Equal(t, []string{"http://localhost:3000", "http://127.0.0.1:3000"}, cfg.CORSOrigins)
	assert.Equal(t, 64, cfg.MaxClients)
	assert.Equal(t, 1024, cfg.EventBuffer)
	assert.Equal(t, time.Duration(0), cfg.Latency)
	assert.Empty(t, cfg.AuthToken)
	require.NoError(t, cfg.Validate())
}

func TestLoadConfigFromEnv(t *testing.T) {
	clearSimulateEnv(t)
	t.Setenv("SIMULATE_ADDR", "0.0.0.0")
	t.Setenv("SIMULATE_PORT", "9090")
	t.Setenv("SIMULATE_UI", "false")
	t.Setenv("SIMULATE_CORS", " http://a.test ,http://b.test,http://a.test ")
	t.Setenv("SIMULATE_MAX_CLIENTS", "12")
	t.Setenv("SIMULATE_EVENT_BUFFER", "256")
	t.Setenv("SIMULATE_LATENCY", "25ms")
	t.Setenv("SIMULATE_AUTH_TOKEN", "secret")

	cfg, err := LoadConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", cfg.Addr)
	assert.Equal(t, 9090, cfg.Port)
	assert.False(t, cfg.UIEnabled)
	assert.Equal(t, []string{"http://a.test", "http://b.test"}, cfg.CORSOrigins)
	assert.Equal(t, 12, cfg.MaxClients)
	assert.Equal(t, 256, cfg.EventBuffer)
	assert.Equal(t, 25*time.Millisecond, cfg.Latency)
	assert.Equal(t, "secret", cfg.AuthToken)
}

func TestLoadConfigInvalidEnv(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		message string
	}{
		{name: "duration", key: "SIMULATE_LATENCY", value: "soon", message: "SIMULATE_LATENCY must be a valid duration"},
		{name: "port", key: "SIMULATE_PORT", value: "eighty", message: "SIMULATE_PORT must be an integer"},
		{name: "max clients", key: "SIMULATE_MAX_CLIENTS", value: "many", message: "SIMULATE_MAX_CLIENTS must be an integer"},
		{name: "event buffer", key: "SIMULATE_EVENT_BUFFER", value: "lots", message: "SIMULATE_EVENT_BUFFER must be an integer"},
		{name: "ui", key: "SIMULATE_UI", value: "maybe", message: "SIMULATE_UI must be a boolean"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSimulateEnv(t)
			t.Setenv(tt.key, tt.value)
			_, err := LoadConfigFromEnv()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

func TestLoadConfigFlagsOverrideEnv(t *testing.T) {
	clearSimulateEnv(t)
	t.Setenv("SIMULATE_PORT", "9000")
	t.Setenv("SIMULATE_UI", "true")

	cfg, err := LoadConfig([]string{
		"--port", "9001",
		"--ui=false",
		"--cors", "http://one.test, http://two.test,http://one.test",
		"--latency", "10ms",
		"--max-clients", "8",
		"--event-buffer", "64",
		"--addr", "127.0.0.2",
		"--auth-token", "flag-token",
	})
	require.NoError(t, err)
	assert.Equal(t, 9001, cfg.Port)
	assert.False(t, cfg.UIEnabled)
	assert.Equal(t, []string{"http://one.test", "http://two.test"}, cfg.CORSOrigins)
	assert.Equal(t, 10*time.Millisecond, cfg.Latency)
	assert.Equal(t, 8, cfg.MaxClients)
	assert.Equal(t, 64, cfg.EventBuffer)
	assert.Equal(t, "127.0.0.2", cfg.Addr)
	assert.Equal(t, "flag-token", cfg.AuthToken)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{name: "port negative", mutate: func(c *Config) { c.Port = -1 }, message: "SIMULATE_PORT must be between 0 and 65535"},
		{name: "port high", mutate: func(c *Config) { c.Port = 70000 }, message: "SIMULATE_PORT must be between 0 and 65535"},
		{name: "addr", mutate: func(c *Config) { c.Addr = "  " }, message: "SIMULATE_ADDR must not be empty"},
		{name: "max clients", mutate: func(c *Config) { c.MaxClients = 0 }, message: "SIMULATE_MAX_CLIENTS must be between 1 and 10000"},
		{name: "event buffer", mutate: func(c *Config) { c.EventBuffer = 1 }, message: "SIMULATE_EVENT_BUFFER must be between 16 and 100000"},
		{name: "latency", mutate: func(c *Config) { c.Latency = -time.Second }, message: "SIMULATE_LATENCY must be between 0s and 30s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Equal(t, "simulate: invalid config: "+tt.message, err.Error())
		})
	}
}
