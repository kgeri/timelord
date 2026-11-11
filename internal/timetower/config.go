// Package timetower implements the Timetower REST service.
package timetower

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"time"

	"timelord/internal/policy"
)

const (
	// SchemaVersion is the version of the schedule/status wire format.
	SchemaVersion = 1

	dateLayout = "2006-01-02"

	// maxLimitMinutes keeps a minute limit convertible to int64 seconds.
	maxLimitMinutes = (1<<63 - 1) / 60
)

// Config is Timetower's static configuration.
type Config struct {
	Timezone              string `json:"timezone"`
	StatusIntervalSeconds int    `json:"status_interval_seconds"`
	OfflineGraceSeconds   int    `json:"offline_grace_seconds"`
	Users                 []User `json:"users"`

	loc         *time.Location
	usersByName map[string]*User
	revision    int64
}

// User is a managed account and its policy.
type User struct {
	Name        string        `json:"name"`
	DisplayName string        `json:"display_name"`
	Policy      *ConfigPolicy `json:"policy"`
}

// ConfigPolicy is a user's configured policy. Each window carries the daily
// limit for its weekday in minutes. Windows on the same weekday must agree on
// the limit; a missing or zero limit means no duration limit.
type ConfigPolicy struct {
	Enabled bool           `json:"enabled"`
	Windows []ConfigWindow `json:"windows"`
}

// ConfigWindow is a permitted interval and the daily limit for its weekday.
type ConfigWindow struct {
	policy.Window
	LimitMinutes int64 `json:"limit_minutes"`
}

// LoadConfig reads and validates a Timetower configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("parse config: unexpected data after JSON object")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	return &config, nil
}

// Validate checks the static configuration and builds lookup tables.
func (c *Config) Validate() error {
	if c.Timezone == "" {
		return fmt.Errorf("timezone is empty")
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("timezone %q: %w", c.Timezone, err)
	}
	c.loc = loc

	if c.StatusIntervalSeconds <= 0 {
		return fmt.Errorf("status_interval_seconds must be positive")
	}
	if c.OfflineGraceSeconds < 0 {
		return fmt.Errorf("offline_grace_seconds must not be negative")
	}

	users := make(map[string]*User, len(c.Users))
	for i := range c.Users {
		user := &c.Users[i]
		if user.Name == "" {
			return fmt.Errorf("users[%d]: name is empty", i)
		}
		if _, ok := users[user.Name]; ok {
			return fmt.Errorf("users[%d]: duplicate user %q", i, user.Name)
		}
		if user.DisplayName == "" {
			user.DisplayName = user.Name
		}
		if err := user.Policy.Validate(); err != nil {
			return fmt.Errorf("user %q: %w", user.Name, err)
		}
		users[user.Name] = user
	}

	c.usersByName = users
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("hash config: %w", err)
	}
	h := fnv.New64a()
	_, _ = h.Write(data)
	c.revision = int64(h.Sum64() >> 1)
	return nil
}

// Location returns the configured policy time zone.
func (c *Config) Location() *time.Location {
	return c.loc
}

// Revision returns the static schedule revision.
func (c *Config) Revision() int64 {
	return c.revision
}

// RevisionForDay returns the schedule revision for one policy day. It changes
// with the configuration and with the day, because the wire policy carries the
// current day's converted daily limit.
func (c *Config) RevisionForDay(day string) int64 {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d:%s", c.revision, day)
	return int64(h.Sum64() >> 1)
}

// UserByName returns the configured user.
func (c *Config) UserByName(name string) (*User, bool) {
	user, ok := c.usersByName[name]
	return user, ok
}

// Validate checks the policy's daily limits and windows.
func (p *ConfigPolicy) Validate() error {
	if p == nil {
		return nil
	}
	limits := make(map[policy.Day]int64, len(p.Windows))
	for i, window := range p.Windows {
		if !window.Day.Valid() {
			return fmt.Errorf("windows[%d]: unknown day %q", i, window.Day)
		}
		if !window.Start.Valid() {
			return fmt.Errorf("windows[%d]: start is out of range", i)
		}
		if !window.End.Valid() {
			return fmt.Errorf("windows[%d]: end is out of range", i)
		}
		if window.End <= window.Start {
			return fmt.Errorf("windows[%d]: end must be later than start", i)
		}
		if window.LimitMinutes < 0 {
			return fmt.Errorf("windows[%d]: limit_minutes must not be negative", i)
		}
		if window.LimitMinutes > maxLimitMinutes {
			return fmt.Errorf("windows[%d]: limit_minutes is too large", i)
		}
		if previous, ok := limits[window.Day]; ok && previous != window.LimitMinutes {
			return fmt.Errorf("windows[%d]: limit_minutes for %s must match its other windows", i, window.Day)
		}
		limits[window.Day] = window.LimitMinutes
	}
	return nil
}

// ForDay returns the API representation of the policy for one weekday. The
// configured minutes are converted to the API's seconds field.
func (p *ConfigPolicy) ForDay(day policy.Day) *Policy {
	if p == nil {
		return nil
	}
	windows := make([]policy.Window, len(p.Windows))
	var minutes int64
	for i, window := range p.Windows {
		windows[i] = window.Window
		if window.Day == day {
			minutes = window.LimitMinutes
		}
	}
	return &Policy{
		Enabled:           p.Enabled,
		DailyLimitSeconds: minutes * 60,
		Windows:           windows,
	}
}

// DayWindow returns the envelope of the configured windows for one weekday.
// The start is the earliest start and the end is the latest end that day.
func (p *ConfigPolicy) DayWindow(day policy.Day) (policy.Window, bool) {
	if p == nil {
		return policy.Window{}, false
	}
	var window policy.Window
	found := false
	for _, candidate := range p.Windows {
		if candidate.Day != day {
			continue
		}
		if !found {
			window = candidate.Window
			found = true
			continue
		}
		if candidate.Start < window.Start {
			window.Start = candidate.Start
		}
		if candidate.End > window.End {
			window.End = candidate.End
		}
	}
	return window, found
}

// DayWindowOrDefault returns the envelope of the configured windows for one
// weekday. A weekday without windows defaults to 00:00-23:59 so an override
// can create a window.
func (p *ConfigPolicy) DayWindowOrDefault(day policy.Day) policy.Window {
	if window, ok := p.DayWindow(day); ok {
		return window
	}
	return policy.Window{Day: day, Start: 0, End: policy.MinutesPerDay - 1}
}

// InsideWindow reports whether now is inside one of the policy's windows.
func (p *Policy) InsideWindow(now time.Time, loc *time.Location) bool {
	if p == nil || !p.Enabled {
		return false
	}
	local := now.In(loc)
	day := weekdayDays[int(local.Weekday())]
	clock := policy.Clock(local.Hour()*60 + local.Minute())
	for _, window := range p.Windows {
		if window.Day == day && window.Start <= clock && clock < window.End {
			return true
		}
	}
	return false
}

// Allowed reports whether the policy permits use at now after usedSeconds.
func (p *Policy) Allowed(now time.Time, usedSeconds int64, loc *time.Location) bool {
	if p == nil || !p.Enabled || !p.InsideWindow(now, loc) {
		return false
	}
	return p.DailyLimitSeconds == 0 || usedSeconds < p.DailyLimitSeconds
}

func policyDay(now time.Time, loc *time.Location) policy.Day {
	return weekdayDays[int(now.In(loc).Weekday())]
}

var weekdayDays = [...]policy.Day{
	policy.Sunday,
	policy.Monday,
	policy.Tuesday,
	policy.Wednesday,
	policy.Thursday,
	policy.Friday,
	policy.Saturday,
}
