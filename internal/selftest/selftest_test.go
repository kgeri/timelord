package selftest

import (
	"errors"
	"testing"

	"timelord/internal/process"
)

func TestRunPasses(t *testing.T) {
	cfg := Config{
		Privileged: true,
		List: func() ([]process.Process, error) {
			return []process.Process{{Executable: "/bin/bash"}}, nil
		},
		Listen: func() error { return nil },
	}

	if err := Run(cfg); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
}

func TestRunFails(t *testing.T) {
	processes := func() ([]process.Process, error) {
		return []process.Process{{Executable: "/bin/bash"}}, nil
	}
	noop := func() error { return nil }

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "not privileged",
			cfg:  Config{Privileged: false, List: processes, Listen: noop},
		},
		{
			name: "list error",
			cfg: Config{
				Privileged: true,
				List:       func() ([]process.Process, error) { return nil, errors.New("boom") },
				Listen:     noop,
			},
		},
		{
			name: "no processes",
			cfg: Config{
				Privileged: true,
				List:       func() ([]process.Process, error) { return nil, nil },
				Listen:     noop,
			},
		},
		{
			name: "no readable executables",
			cfg: Config{
				Privileged: true,
				List:       func() ([]process.Process, error) { return []process.Process{{Name: "svchost"}}, nil },
				Listen:     noop,
			},
		},
		{
			name: "listen error",
			cfg: Config{
				Privileged: true,
				List:       processes,
				Listen:     func() error { return errors.New("address already in use") },
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Run(tt.cfg); err == nil {
				t.Error("Run() = nil, want an error")
			}
		})
	}
}
