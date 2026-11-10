// Package timetower implements the Timetower REST service.
package timetower

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"os"
	"time"

	"timelord/internal/schedule"
)

const (
	// SchemaVersion is the version of the schedule/status wire format.
	SchemaVersion = 1

	dateLayout = "2006-01-02"
)

// Config is Timetower's static configuration.
type Config struct {
	Timezone              string     `json:"timezone"`
	StatusIntervalSeconds int        `json:"status_interval_seconds"`
	OfflineGraceSeconds   int        `json:"offline_grace_seconds"`
	Subjects              []Subject  `json:"subjects"`
	Enforcers             []Enforcer `json:"enforcers"`

	loc           *time.Location
	subjectsByID  map[string]*Subject
	enforcersByIP map[string]*Enforcer
	revision      int64
}

// Subject is a managed identity and its policy.
type Subject struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	Policy      *Policy `json:"policy"`
}

// Policy is a subject's permitted windows and daily limit.
type Policy struct {
	Enabled           bool              `json:"enabled"`
	DailyLimitSeconds int64             `json:"daily_limit_seconds"`
	Windows           []schedule.Window `json:"windows"`
}

// Enforcer is one managed host and the accounts it enforces.
type Enforcer struct {
	IP       string    `json:"ip"`
	Name     string    `json:"name"`
	Accounts []Account `json:"accounts"`
}

// Account maps a subject to an OS account on an Enforcer.
type Account struct {
	Subject string `json:"subject"`
	Account string `json:"account"`
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

	subjects := make(map[string]*Subject, len(c.Subjects))
	for i := range c.Subjects {
		subject := &c.Subjects[i]
		if subject.ID == "" {
			return fmt.Errorf("subjects[%d]: id is empty", i)
		}
		if _, ok := subjects[subject.ID]; ok {
			return fmt.Errorf("subjects[%d]: duplicate subject %q", i, subject.ID)
		}
		if subject.DisplayName == "" {
			subject.DisplayName = subject.ID
		}
		if err := subject.Policy.Validate(); err != nil {
			return fmt.Errorf("subject %q: %w", subject.ID, err)
		}
		subjects[subject.ID] = subject
	}

	enforcersByIP := make(map[string]*Enforcer, len(c.Enforcers))
	names := make(map[string]bool, len(c.Enforcers))
	for i := range c.Enforcers {
		enforcer := &c.Enforcers[i]
		ip := net.ParseIP(enforcer.IP)
		if ip == nil {
			return fmt.Errorf("enforcers[%d]: invalid IP address %q", i, enforcer.IP)
		}
		enforcer.IP = ip.String()
		if _, ok := enforcersByIP[enforcer.IP]; ok {
			return fmt.Errorf("enforcers[%d]: duplicate IP address %q", i, enforcer.IP)
		}
		if enforcer.Name == "" {
			enforcer.Name = enforcer.IP
		}
		if names[enforcer.Name] {
			return fmt.Errorf("enforcers[%d]: duplicate name %q", i, enforcer.Name)
		}
		names[enforcer.Name] = true

		subjectsSeen := make(map[string]bool, len(enforcer.Accounts))
		accountsSeen := make(map[string]bool, len(enforcer.Accounts))
		for j, account := range enforcer.Accounts {
			if account.Subject == "" {
				return fmt.Errorf("enforcer %q: accounts[%d]: subject is empty", enforcer.Name, j)
			}
			if _, ok := subjects[account.Subject]; !ok {
				return fmt.Errorf("enforcer %q: accounts[%d]: unknown subject %q", enforcer.Name, j, account.Subject)
			}
			if subjectsSeen[account.Subject] {
				return fmt.Errorf("enforcer %q: duplicate subject %q", enforcer.Name, account.Subject)
			}
			subjectsSeen[account.Subject] = true

			if account.Account == "" {
				return fmt.Errorf("enforcer %q: accounts[%d]: account is empty", enforcer.Name, j)
			}
			if accountsSeen[account.Account] {
				return fmt.Errorf("enforcer %q: duplicate account %q", enforcer.Name, account.Account)
			}
			accountsSeen[account.Account] = true
		}
		enforcersByIP[enforcer.IP] = enforcer
	}

	c.subjectsByID = subjects
	c.enforcersByIP = enforcersByIP

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

// EnforcerByIP returns the Enforcer configured for ip.
func (c *Config) EnforcerByIP(ip string) (*Enforcer, bool) {
	enforcer, ok := c.enforcersByIP[ip]
	return enforcer, ok
}

// SubjectByID returns the subject with the given ID.
func (c *Config) SubjectByID(id string) (*Subject, bool) {
	subject, ok := c.subjectsByID[id]
	return subject, ok
}

// Validate checks the policy's daily limit and windows.
func (p *Policy) Validate() error {
	if p == nil {
		return nil
	}
	if p.DailyLimitSeconds < 0 {
		return fmt.Errorf("daily_limit_seconds must not be negative")
	}
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
	}
	return nil
}

// InsideWindow reports whether now is inside one of the policy's windows.
func (p *Policy) InsideWindow(now time.Time, loc *time.Location) bool {
	if p == nil || !p.Enabled {
		return false
	}
	local := now.In(loc)
	day := weekdayDays[int(local.Weekday())]
	clock := schedule.Clock(local.Hour()*60 + local.Minute())
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

var weekdayDays = [...]schedule.Day{
	schedule.Sunday,
	schedule.Monday,
	schedule.Tuesday,
	schedule.Wednesday,
	schedule.Thursday,
	schedule.Friday,
	schedule.Saturday,
}
