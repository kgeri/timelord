package timetower

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"timelord/internal/policy"
)

func TestLoadConfig(t *testing.T) {
	path := writeConfig(t, testConfigJSON)

	config, err := LoadConfig(path)
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
	if user.Policy.DailyLimitSeconds != 3600 {
		t.Errorf("DailyLimitSeconds = %d, want 3600", user.Policy.DailyLimitSeconds)
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
	if user.Policy == nil || user.Policy.DailyLimitSeconds != 3600 {
		t.Fatalf("default daily limit = %+v, want 3600 seconds", user.Policy)
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
	tests := map[string]string{
		"empty timezone": `{
			"timezone":"",
			"status_interval_seconds":60,
			"users":[]
		}`,
		"bad timezone": `{
			"timezone":"Not/AZone",
			"status_interval_seconds":60,
			"users":[]
		}`,
		"zero status interval": `{
			"timezone":"UTC",
			"status_interval_seconds":0,
			"users":[]
		}`,
		"duplicate user": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"users":[{"name":"dani"},{"name":"dani"}]
		}`,
		"bad window": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"users":[{"name":"dani","policy":{"enabled":true,"daily_limit_seconds":0,"windows":[{"day":"mon","start":"19:00","end":"08:00"}]}}]
		}`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			var config Config
			if err := json.Unmarshal([]byte(content), &config); err != nil {
				t.Fatalf("unmarshal config: %v", err)
			}
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
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

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "timetower.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const testConfigJSON = `{
	"timezone": "UTC",
	"status_interval_seconds": 60,
	"offline_grace_seconds": 3600,
	"users": [
		{
			"name": "dani",
			"display_name": "Dani",
			"policy": {
				"enabled": true,
				"daily_limit_seconds": 3600,
				"windows": [
					{"day": "mon", "start": "00:00", "end": "23:59"}
				]
			}
		}
	]
}`
