package timetower

import (
	"path/filepath"
	"testing"
	"time"

	"timelord/internal/policy"
)

func TestLoadConfig(t *testing.T) {
	config, err := LoadConfig(testdataPath("config", "valid.json"))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", config.Timezone)
	}
	if config.Revision() == 0 {
		t.Error("Revision() = 0, want non-zero")
	}

	user, ok := config.UserByName("dani")
	if !ok {
		t.Fatal("UserByName(dani) not found")
	}
	if user.DisplayName != "Dani" {
		t.Errorf("DisplayName = %q, want Dani", user.DisplayName)
	}
	if user.Policy == nil {
		t.Fatal("user.Policy = nil")
	}
	if got := mondayLimitMinutes(user.Policy); got != 60 {
		t.Errorf("Monday daily limit = %d, want 60 minutes", got)
	}
}

func TestLoadDefaultConfig(t *testing.T) {
	config, err := LoadConfig(filepath.Join("..", "..", "deploy", "timetower", "config-default.json"))
	if err != nil {
		t.Fatalf("LoadConfig(default) error = %v", err)
	}
	user, ok := config.UserByName("dani")
	if !ok {
		t.Fatal("default user not found")
	}
	if user.Policy == nil || mondayLimitMinutes(user.Policy) != 60 {
		t.Fatalf("default daily limit = %+v, want 60 minutes", user.Policy)
	}
	for _, window := range user.Policy.Windows {
		if window.Day == policy.Tuesday && window.End.String() != "20:30" {
			t.Errorf("Tuesday end = %s, want 20:30", window.End)
		}
	}
}

func TestLoadConfigMissing(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("LoadConfig(missing) error = nil, want error")
	}
}

func TestConfigValidateRejectsInvalidValues(t *testing.T) {
	files := []string{
		"empty-timezone.json",
		"bad-timezone.json",
		"zero-status-interval.json",
		"duplicate-user.json",
		"bad-window.json",
		"unknown-window-day.json",
		"negative-limit.json",
		"conflicting-limits.json",
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			if _, err := LoadConfig(testdataPath("config", "invalid", file)); err == nil {
				t.Fatal("LoadConfig() error = nil, want error")
			}
		})
	}
}

func TestConfigPolicyForDay(t *testing.T) {
	config, err := LoadConfig(testdataPath("config", "per-day.json"))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	user, ok := config.UserByName("dani")
	if !ok {
		t.Fatal("UserByName(dani) not found")
	}

	if got := user.Policy.ForDay(policy.Monday).DailyLimitSeconds; got != 3600 {
		t.Errorf("Monday DailyLimitSeconds = %d, want 3600", got)
	}
	if got := user.Policy.ForDay(policy.Tuesday).DailyLimitSeconds; got != 5400 {
		t.Errorf("Tuesday DailyLimitSeconds = %d, want 5400", got)
	}
	if got := user.Policy.ForDay(policy.Wednesday).DailyLimitSeconds; got != 0 {
		t.Errorf("Wednesday DailyLimitSeconds = %d, want 0", got)
	}
}

func TestPolicyInsideWindow(t *testing.T) {
	policy := &Policy{
		Enabled:           true,
		DailyLimitSeconds: 3600,
		Windows: []policy.Window{
			{Day: policy.Monday, Start: 8 * 60, End: 12 * 60},
			{Day: policy.Monday, Start: 13 * 60, End: 19 * 60},
		},
	}
	location := time.UTC
	at := func(hour, minute int) time.Time {
		return time.Date(2024, time.June, 3, hour, minute, 0, 0, time.UTC)
	}

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "before window", now: at(7, 59), want: false},
		{name: "window start", now: at(8, 0), want: true},
		{name: "between windows", now: at(12, 30), want: false},
		{name: "second window", now: at(18, 0), want: true},
		{name: "window end", now: at(19, 0), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := policy.InsideWindow(test.now, location); got != test.want {
				t.Errorf("InsideWindow() = %v, want %v", got, test.want)
			}
		})
	}
}

func mondayLimitMinutes(p *ConfigPolicy) int64 {
	for _, window := range p.Windows {
		if window.Day == policy.Monday {
			return window.LimitMinutes
		}
	}
	return 0
}
