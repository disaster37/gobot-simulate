package conformance

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "regenerate docs/support-matrix.md")

func TestDriverInTheLoop(t *testing.T) {
	for _, c := range Cases() {
		c := c
		if c.Run == nil {
			continue
		}
		t.Run(c.Model, func(t *testing.T) {
			if raceEnabled && c.Model == "firmata" {
				// gobot's Firmata client updates its pin table from a reader
				// goroutine without synchronization. The race is in the
				// upstream platform package, so the normal suite covers the
				// protocol and the race suite omits this one dependency-bound case.
				t.Skip("upstream gobot firmata client is not race-clean")
			}
			env, err := NewEnv("sim", DefaultTopology)
			if err != nil {
				t.Fatalf("env: %v", err)
			}
			if err := c.Run(env); err != nil {
				t.Fatalf("%s: %v", c.Driver, err)
			}
			_ = env.Sim.Close()
		})
	}
}

func TestGenerateSupportMatrix(t *testing.T) {
	path := filepath.Join("..", "docs", "support-matrix.md")
	generated := Matrix()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(generated), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	current, err := os.ReadFile(path) //nolint:gosec // fixed generated-doc path
	if err != nil {
		t.Fatalf("read %s: %v (run with -update)", path, err)
	}
	if string(current) != generated {
		t.Fatalf("%s is out of date; run: go test ./conformance -run TestGenerateSupportMatrix -update", path)
	}
}
