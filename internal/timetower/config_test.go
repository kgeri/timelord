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

	enforcer, ok := config.EnforcerByIP("192.0.2.10")
	if !ok {
		t.Fatal("EnforcerByIP(192.0.2.10) not found")
	}
	if enforcer.Name != "test-pc" {
		t.Errorf("enforcer.Name = %q, want test-pc", enforcer.Name)
	}
	subject, ok := config.SubjectByID("dani")
	if !ok {
		t.Fatal("SubjectByID(dani) not found")
	}
	if subject.Policy == nil {
		t.Fatal("subject.Policy = nil")
	}
	if subject.Policy.DailyLimitSeconds != 3600 {
		t.Errorf("DailyLimitSeconds = %d, want 3600", subject.Policy.DailyLimitSeconds)
	}
}

func TestLoadDefaultConfig(t *testing.T) {
	config, err := LoadConfig(filepath.Join("..", "..", "deploy", "timetower", "config-default.json"))
	if err != nil {
		t.Fatalf("LoadConfig(default) error = %v", err)
	}
	if _, ok := config.EnforcerByIP("192.168.3.22"); !ok {
		t.Fatal("default enforcer not found")
	}
	subject, ok := config.SubjectByID("dani")
	if !ok {
		t.Fatal("default subject not found")
	}
	if subject.Policy == nil || subject.Policy.DailyLimitSeconds != 3600 {
		t.Fatalf("default daily limit = %+v, want 3600 seconds", subject.Policy)
	}
	for _, window := range subject.Policy.Windows {
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
			"subjects":[],
			"enforcers":[]
		}`,
		"bad timezone": `{
			"timezone":"Not/AZone",
			"status_interval_seconds":60,
			"subjects":[],
			"enforcers":[]
		}`,
		"zero status interval": `{
			"timezone":"UTC",
			"status_interval_seconds":0,
			"subjects":[],
			"enforcers":[]
		}`,
		"duplicate subject": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"subjects":[{"id":"dani"},{"id":"dani"}],
			"enforcers":[]
		}`,
		"unknown account subject": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"subjects":[],
			"enforcers":[{"ip":"192.0.2.10","accounts":[{"subject":"dani","account":"dani"}]}]
		}`,
		"duplicate IP": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"subjects":[],
			"enforcers":[{"ip":"192.0.2.10"},{"ip":"192.0.2.10"}]
		}`,
		"bad window": `{
			"timezone":"UTC",
			"status_interval_seconds":60,
			"subjects":[{"id":"dani","policy":{"enabled":true,"daily_limit_seconds":0,"windows":[{"day":"mon","start":"19:00","end":"08:00"}]}}],
			"enforcers":[]
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
	"subjects": [
		{
			"id": "dani",
			"display_name": "Dani",
			"policy": {
				"enabled": true,
				"daily_limit_seconds": 3600,
				"windows": [
					{"day": "mon", "start": "00:00", "end": "23:59"}
				]
			}
		}
	],
	"enforcers": [
		{
			"ip": "192.0.2.10",
			"name": "test-pc",
			"accounts": [
				{"subject": "dani", "account": "dani"}
			]
		}
	]
}`
