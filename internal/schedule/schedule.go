// Package schedule defines the per-user time schedule that TimeLord enforces.
package schedule

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

// FileName is the default schedule file name in the user's home directory.
const FileName = "timelord-schedule.json"

// MinutesPerDay is the number of minutes in a day.
const MinutesPerDay = 24 * 60

// Config is the root of the schedule file.
type Config struct {
	// Users holds one policy for each managed user.
	Users []Policy `json:"users"`
}

// Policy is the schedule for one user. Both the windows and the daily limit
// must allow access.
type Policy struct {
	// User is the account name, as reported by the process lister.
	User string `json:"user"`
	// DailyLimitMinutes is the maximum active time per local day. Zero means
	// no duration limit.
	DailyLimitMinutes int `json:"daily_limit_minutes"`
	// Windows are the permitted intervals. A day with no window is denied.
	Windows []Window `json:"windows"`
}

// Window is a permitted interval on one weekday. It does not cross midnight.
type Window struct {
	Day   Day   `json:"day"`
	Start Clock `json:"start"`
	End   Clock `json:"end"`
}

// Day is a weekday name in a schedule.
type Day string

// Weekday names used in the schedule file.
const (
	Monday    Day = "mon"
	Tuesday   Day = "tue"
	Wednesday Day = "wed"
	Thursday  Day = "thu"
	Friday    Day = "fri"
	Saturday  Day = "sat"
	Sunday    Day = "sun"
)

var dayNames = [...]Day{Sunday, Monday, Tuesday, Wednesday, Thursday, Friday, Saturday}

// Valid reports whether d is a known weekday name.
func (d Day) Valid() bool {
	switch d {
	case Monday, Tuesday, Wednesday, Thursday, Friday, Saturday, Sunday:
		return true
	default:
		return false
	}
}

// Clock is a time of day, stored as minutes since midnight.
type Clock int

// ParseClock parses a 24-hour HH:MM value.
func ParseClock(s string) (Clock, error) {
	if len(s) != 5 || s[2] != ':' {
		return 0, fmt.Errorf("clock %q must use HH:MM", s)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("clock %q must use HH:MM", s)
		}
	}

	hour, _ := strconv.Atoi(s[:2])
	minute, _ := strconv.Atoi(s[3:])
	if hour > 23 || minute > 59 {
		return 0, fmt.Errorf("clock %q is out of range", s)
	}
	return Clock(hour*60 + minute), nil
}

// String returns the clock as HH:MM.
func (c Clock) String() string {
	return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60)
}

// Valid reports whether c is a time between 00:00 and 23:59.
func (c Clock) Valid() bool {
	return c >= 0 && c < MinutesPerDay
}

// UnmarshalJSON parses an HH:MM string.
func (c *Clock) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	clock, err := ParseClock(s)
	if err != nil {
		return err
	}
	*c = clock
	return nil
}

// MarshalJSON writes the clock as an HH:MM string.
func (c Clock) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.String())
}

// Load reads and validates a schedule file. A caller that treats a missing file
// as an empty schedule should check for fs.ErrNotExist.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse schedule: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate schedule: %w", err)
	}
	return &config, nil
}

// Validate checks the policy names, limits, and windows.
func (c *Config) Validate() error {
	users := make(map[string]struct{}, len(c.Users))
	for i := range c.Users {
		policy := &c.Users[i]
		if policy.User == "" {
			return fmt.Errorf("users[%d]: user is empty", i)
		}
		if _, ok := users[policy.User]; ok {
			return fmt.Errorf("users[%d]: duplicate user %q", i, policy.User)
		}
		users[policy.User] = struct{}{}

		if policy.DailyLimitMinutes < 0 {
			return fmt.Errorf("user %q: daily_limit_minutes must not be negative", policy.User)
		}

		for j := range policy.Windows {
			window := &policy.Windows[j]
			if !window.Day.Valid() {
				return fmt.Errorf("user %q: windows[%d]: unknown day %q", policy.User, j, window.Day)
			}
			if !window.Start.Valid() {
				return fmt.Errorf("user %q: windows[%d]: start is out of range", policy.User, j)
			}
			if !window.End.Valid() {
				return fmt.Errorf("user %q: windows[%d]: end is out of range", policy.User, j)
			}
			if window.End <= window.Start {
				return fmt.Errorf("user %q: windows[%d]: end must be later than start", policy.User, j)
			}
		}
	}
	return nil
}

// ForUser returns the policy for user.
func (c *Config) ForUser(user string) (Policy, bool) {
	if c == nil {
		return Policy{}, false
	}
	for _, policy := range c.Users {
		if policy.User == user {
			return policy, true
		}
	}
	return Policy{}, false
}

// Allowed reports whether user may use the computer at now after used active
// time. A user without a policy is never allowed.
func (c *Config) Allowed(user string, now time.Time, used time.Duration) bool {
	policy, ok := c.ForUser(user)
	if !ok {
		return false
	}
	return policy.Allowed(now, used)
}

// Allowed reports whether the policy permits use at now after used active time.
func (p Policy) Allowed(now time.Time, used time.Duration) bool {
	local := now.In(time.Local)
	if !p.inWindow(local) {
		return false
	}
	if p.DailyLimitMinutes > 0 && used >= time.Duration(p.DailyLimitMinutes)*time.Minute {
		return false
	}
	return true
}

// inWindow reports whether now falls inside one of the policy's windows.
func (p Policy) inWindow(now time.Time) bool {
	day := dayNames[int(now.Weekday())]
	clock := Clock(now.Hour()*60 + now.Minute())
	for _, window := range p.Windows {
		if window.Day == day && window.Start <= clock && clock < window.End {
			return true
		}
	}
	return false
}
