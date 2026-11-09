package schedule

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	path := writeFile(t, `{
		"users": [
			{
				"user": "dani",
				"daily_limit_minutes": 60,
				"windows": [
					{"day": "mon", "start": "08:00", "end": "19:00"},
					{"day": "tue", "start": "09:30", "end": "12:00"}
				]
			}
		]
	}`)

	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(config.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(config.Users))
	}

	policy, ok := config.ForUser("dani")
	if !ok {
		t.Fatal("dani not found")
	}
	if policy.DailyLimitMinutes != 60 {
		t.Errorf("DailyLimitMinutes = %d, want 60", policy.DailyLimitMinutes)
	}
	if len(policy.Windows) != 2 {
		t.Fatalf("len(Windows) = %d, want 2", len(policy.Windows))
	}
	if got := policy.Windows[0].Start.String(); got != "08:00" {
		t.Errorf("start = %s, want 08:00", got)
	}
	if got := policy.Windows[1].End.String(); got != "12:00" {
		t.Errorf("end = %s, want 12:00", got)
	}
}

func TestLoadMissingReturnsNotExist(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load() error = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadSampleDaniConfig(t *testing.T) {
	config, err := Load(filepath.Join("..", "..", "timelord-schedule.dani.json"))
	if err != nil {
		t.Fatalf("Load(sample) error = %v", err)
	}

	policy, ok := config.ForUser("dani")
	if !ok {
		t.Fatal("dani not found in sample config")
	}
	if policy.DailyLimitMinutes != 60 {
		t.Errorf("DailyLimitMinutes = %d, want 60", policy.DailyLimitMinutes)
	}
	if len(policy.Windows) != 7 {
		t.Fatalf("len(Windows) = %d, want 7", len(policy.Windows))
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	tests := map[string]string{
		"empty user":       `{"users":[{"user":"","windows":[]}]}`,
		"duplicate user":   `{"users":[{"user":"dani"},{"user":"dani"}]}`,
		"negative limit":   `{"users":[{"user":"dani","daily_limit_minutes":-1}]}`,
		"unknown day":      `{"users":[{"user":"dani","windows":[{"day":"monday","start":"08:00","end":"19:00"}]}]}`,
		"bad start":        `{"users":[{"user":"dani","windows":[{"day":"mon","start":"8:00","end":"19:00"}]}]}`,
		"bad end minute":   `{"users":[{"user":"dani","windows":[{"day":"mon","start":"08:00","end":"19:60"}]}]}`,
		"end equals start": `{"users":[{"user":"dani","windows":[{"day":"mon","start":"08:00","end":"08:00"}]}]}`,
		"end before start": `{"users":[{"user":"dani","windows":[{"day":"mon","start":"19:00","end":"08:00"}]}]}`,
		"unknown field":    `{"users":[{"user":"dani","window":[]}]}`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeFile(t, content)); err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestAllowed(t *testing.T) {
	config := &Config{Users: []Policy{{
		User:              "dani",
		DailyLimitMinutes: 60,
		Windows: []Window{
			{Day: Monday, Start: 8 * 60, End: 12 * 60},
			{Day: Monday, Start: 13 * 60, End: 19 * 60},
		},
	}}}

	// 2024-06-03 is a Monday.
	at := func(hour, minute int) time.Time {
		return time.Date(2024, time.June, 3, hour, minute, 0, 0, time.Local)
	}

	tests := []struct {
		name string
		now  time.Time
		used time.Duration
		want bool
	}{
		{name: "before window", now: at(7, 59), want: false},
		{name: "window start", now: at(8, 0), want: true},
		{name: "inside first window", now: at(10, 0), want: true},
		{name: "between windows", now: at(12, 30), want: false},
		{name: "inside second window", now: at(18, 0), want: true},
		{name: "window end", now: at(19, 0), want: false},
		{name: "after window", now: at(20, 0), want: false},
		{name: "under limit", now: at(10, 0), used: 59*time.Minute + 59*time.Second, want: true},
		{name: "at limit", now: at(10, 0), used: 60 * time.Minute, want: false},
		{name: "over limit", now: at(10, 0), used: 61 * time.Minute, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := config.Allowed("dani", test.now, test.used); got != test.want {
				t.Errorf("Allowed() = %v, want %v", got, test.want)
			}
		})
	}

	tuesday := time.Date(2024, time.June, 4, 10, 0, 0, 0, time.Local)
	if config.Allowed("dani", tuesday, 0) {
		t.Error("Allowed() on a day without a window = true, want false")
	}
	if config.Allowed("someone-else", at(10, 0), 0) {
		t.Error("Allowed() for a user without a policy = true, want false")
	}
}

func TestAllowedWithoutDailyLimit(t *testing.T) {
	policy := Policy{
		User:    "dani",
		Windows: []Window{{Day: Monday, Start: 8 * 60, End: 19 * 60}},
	}
	monday := time.Date(2024, time.June, 3, 12, 0, 0, 0, time.Local)

	if !policy.Allowed(monday, 24*time.Hour) {
		t.Error("Allowed() with unlimited daily time = false, want true")
	}
}

func TestParseClock(t *testing.T) {
	tests := map[string]struct {
		want Clock
		ok   bool
	}{
		"00:00": {want: 0, ok: true},
		"08:05": {want: 8*60 + 5, ok: true},
		"23:59": {want: 23*60 + 59, ok: true},
		"24:00": {ok: false},
		"7:00":  {ok: false},
		"08:60": {ok: false},
		"+8:00": {ok: false},
	}

	for input, test := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := ParseClock(input)
			if test.ok && err != nil {
				t.Fatalf("ParseClock() error = %v", err)
			}
			if !test.ok {
				if err == nil {
					t.Fatal("ParseClock() error = nil, want error")
				}
				return
			}
			if got != test.want {
				t.Errorf("ParseClock() = %v, want %v", got, test.want)
			}
		})
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "schedule.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write schedule: %v", err)
	}
	return path
}
