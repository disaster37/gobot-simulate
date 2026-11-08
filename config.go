package simulate

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds simulator configuration. Precedence is explicit option/flag,
// then environment variable, then default.
type Config struct {
	Addr        string        `json:"addr"`
	Port        int           `json:"port"`
	UIEnabled   bool          `json:"uiEnabled"`
	CORSOrigins []string      `json:"corsOrigins"`
	MaxClients  int           `json:"maxClients"`
	EventBuffer int           `json:"eventBuffer"`
	Latency     time.Duration `json:"latency"`
	AuthToken   string        `json:"authToken"`
	Topology    string        `json:"topology,omitempty"`
	TraceBuffer int           `json:"traceBuffer"`
	AREST       bool          `json:"arestEnabled"`
}

// DefaultConfig returns the documented default configuration.
func DefaultConfig() Config {
	return Config{
		Addr:        "127.0.0.1",
		Port:        8080,
		UIEnabled:   true,
		CORSOrigins: []string{"http://localhost:3000", "http://127.0.0.1:3000"},
		MaxClients:  64,
		EventBuffer: 1024,
		Latency:     0,
		AuthToken:   "",
		Topology:    "",
		TraceBuffer: 8192,
		AREST:       true,
	}
}

// LoadConfigFromEnv loads defaults overridden by SIMULATE_* environment variables.
func LoadConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadConfig loads defaults, then environment variables, then command-line
// flags from args (excluding the program name).
func LoadConfig(args []string) (Config, error) {
	cfg := DefaultConfig()
	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}

	cors := strings.Join(cfg.CORSOrigins, ",")
	fs := flag.NewFlagSet("gobot-simulate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address")
	fs.IntVar(&cfg.Port, "port", cfg.Port, "listen port (0 = ephemeral)")
	fs.BoolVar(&cfg.UIEnabled, "ui", cfg.UIEnabled, "serve the embedded web UI")
	fs.StringVar(&cors, "cors", cors, "comma-separated CORS allowlist")
	fs.IntVar(&cfg.MaxClients, "max-clients", cfg.MaxClients, "maximum SSE clients")
	fs.IntVar(&cfg.EventBuffer, "event-buffer", cfg.EventBuffer, "SSE replay buffer size")
	fs.DurationVar(&cfg.Latency, "latency", cfg.Latency, "simulated I/O latency")
	fs.StringVar(&cfg.AuthToken, "auth-token", cfg.AuthToken, "optional bearer token")
	fs.StringVar(&cfg.Topology, "topology", cfg.Topology, "path to a topology YAML/JSON file")
	fs.IntVar(&cfg.TraceBuffer, "trace-buffer", cfg.TraceBuffer, "I/O trace ring size")
	fs.BoolVar(&cfg.AREST, "arest", cfg.AREST, "serve the aREST emulation")
	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("simulate: invalid config: %w", err)
	}
	cfg.CORSOrigins = parseCORS(cors)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks configuration bounds and returns documented error messages.
func (c Config) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return errors.New("simulate: invalid config: SIMULATE_PORT must be between 0 and 65535")
	}
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("simulate: invalid config: SIMULATE_ADDR must not be empty")
	}
	if c.MaxClients < 1 || c.MaxClients > 10000 {
		return errors.New("simulate: invalid config: SIMULATE_MAX_CLIENTS must be between 1 and 10000")
	}
	if c.EventBuffer < 16 || c.EventBuffer > 100000 {
		return errors.New("simulate: invalid config: SIMULATE_EVENT_BUFFER must be between 16 and 100000")
	}
	if c.Latency < 0 || c.Latency > 30*time.Second {
		return errors.New("simulate: invalid config: SIMULATE_LATENCY must be between 0s and 30s")
	}
	if c.TraceBuffer < 16 || c.TraceBuffer > 1000000 {
		return errors.New("simulate: invalid config: SIMULATE_TRACE_BUFFER must be between 16 and 1000000")
	}
	return nil
}

func applyEnv(cfg *Config) error {
	if raw, ok := os.LookupEnv("SIMULATE_ADDR"); ok {
		cfg.Addr = strings.TrimSpace(raw)
	}
	if raw, ok := os.LookupEnv("SIMULATE_PORT"); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_PORT must be an integer: %w", err)
		}
		cfg.Port = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_UI"); ok {
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_UI must be a boolean: %w", err)
		}
		cfg.UIEnabled = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_CORS"); ok {
		cfg.CORSOrigins = parseCORS(raw)
	}
	if raw, ok := os.LookupEnv("SIMULATE_MAX_CLIENTS"); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_MAX_CLIENTS must be an integer: %w", err)
		}
		cfg.MaxClients = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_EVENT_BUFFER"); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_EVENT_BUFFER must be an integer: %w", err)
		}
		cfg.EventBuffer = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_LATENCY"); ok {
		value, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_LATENCY must be a valid duration: %w", err)
		}
		cfg.Latency = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_AUTH_TOKEN"); ok {
		cfg.AuthToken = raw
	}
	if raw, ok := os.LookupEnv("SIMULATE_TOPOLOGY"); ok {
		cfg.Topology = strings.TrimSpace(raw)
	}
	if raw, ok := os.LookupEnv("SIMULATE_TRACE_BUFFER"); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_TRACE_BUFFER must be an integer: %w", err)
		}
		cfg.TraceBuffer = value
	}
	if raw, ok := os.LookupEnv("SIMULATE_AREST"); ok {
		value, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("simulate: invalid config: SIMULATE_AREST must be a boolean: %w", err)
		}
		cfg.AREST = value
	}
	return nil
}

func parseCORS(raw string) []string {
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return dedupeStrings(parts)
}
