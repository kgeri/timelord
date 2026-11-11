package timetower

import (
	"encoding/csv"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"timelord/internal/policy"
)

// Store holds usage counters and the latest in-memory status per hostname and
// user. Only usage is persisted.
type Store struct {
	mu       sync.Mutex
	saveMu   sync.Mutex
	path     string
	usage    map[usageKey]int64
	status   map[statusKey]StatusRecord
	override map[overrideKey]DayOverride
}

// StatusRecord is the latest status received for one hostname and user.
type StatusRecord struct {
	Hostname   string
	User       string
	ReceivedAt time.Time
	Request    StatusRequest
}

type usageKey struct {
	hostname string
	user     string
	day      string
}

type statusKey struct {
	hostname string
	user     string
}

type overrideKey struct {
	user string
	day  string
}

// DayOverride replaces any of one day's window boundaries or daily limit.
// A nil field keeps the configured value.
type DayOverride struct {
	Start        *policy.Clock `json:"start,omitempty"`
	End          *policy.Clock `json:"end,omitempty"`
	LimitMinutes *int64        `json:"limit_minutes,omitempty"`
}

func (o DayOverride) empty() bool {
	return o.Start == nil && o.End == nil && o.LimitMinutes == nil
}

// UsageRecord is one row in the usage CSV.
type UsageRecord struct {
	Date        string
	User        string
	Hostname    string
	UsedSeconds int64
}

// NewStore creates a store and loads usage from the CSV at path. A missing CSV
// starts with empty usage. An empty path keeps usage in memory only.
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:     path,
		usage:    make(map[usageKey]int64),
		status:   make(map[statusKey]StatusRecord),
		override: make(map[overrideKey]DayOverride),
	}
	if path == "" {
		return store, nil
	}

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return store, nil
	} else if err != nil {
		return nil, err
	}

	if err := store.loadCSV(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) loadCSV() error {
	file, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return err
	}
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		if i == 0 && strings.EqualFold(strings.TrimSpace(row[0]), "date") {
			continue
		}
		if len(row) != 4 {
			return fmt.Errorf("row %d: expected date,user,hostname,used_seconds", i+1)
		}
		date := strings.TrimSpace(row[0])
		user := strings.TrimSpace(row[1])
		hostname := strings.TrimSpace(row[2])
		used, err := strconv.ParseInt(strings.TrimSpace(row[3]), 10, 64)
		if err != nil || used < 0 || user == "" || hostname == "" {
			return fmt.Errorf("row %d: invalid usage record", i+1)
		}
		if _, err := time.Parse(dateLayout, date); err != nil {
			return fmt.Errorf("row %d: invalid date %q", i+1, date)
		}
		key := usageKey{hostname: hostname, user: user, day: date}
		if used > s.usage[key] {
			s.usage[key] = used
		}
	}
	return nil
}

// RecordUsage stores used seconds for one hostname, user, and day. The counter
// is monotonic; a lower value never replaces a higher one.
func (s *Store) RecordUsage(hostname, user, day string, usedSeconds int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := usageKey{hostname: hostname, user: user, day: day}
	if usedSeconds > s.usage[key] {
		s.usage[key] = usedSeconds
	}
}

// Usage returns the stored counter for one hostname, user, and day.
func (s *Store) Usage(hostname, user, day string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage[usageKey{hostname: hostname, user: user, day: day}]
}

// AggregateUsage returns the sum of all hostnames' counters for a user/day.
func (s *Store) AggregateUsage(user, day string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for key, used := range s.usage {
		if key.user == user && key.day == day {
			total += used
		}
	}
	return total
}

// SetOverride stores the current-day override for one user and day.
// Overrides are in-memory and start empty after a restart.
func (s *Store) SetOverride(user, day string, override DayOverride) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.override {
		if key.user == user && key.day < day {
			delete(s.override, key)
		}
	}
	s.override[overrideKey{user: user, day: day}] = override
}

// ClearOverride removes the current-day override for one user and day.
func (s *Store) ClearOverride(user, day string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.override, overrideKey{user: user, day: day})
}

// Override returns the current-day override for one user and day, if set.
func (s *Store) Override(user, day string) (DayOverride, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	override, ok := s.override[overrideKey{user: user, day: day}]
	return override, ok
}

// OverridesRevision returns a stable digest of one day's overrides. It
// changes whenever an override for the day is set or cleared.
func (s *Store) OverridesRevision(day string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	users := make([]string, 0, len(s.override))
	for key := range s.override {
		if key.day == day {
			users = append(users, key.user)
		}
	}
	sort.Strings(users)

	h := fnv.New64a()
	for _, user := range users {
		override := s.override[overrideKey{user: user, day: day}]
		start, end, limit := "-", "-", "-"
		if override.Start != nil {
			start = override.Start.String()
		}
		if override.End != nil {
			end = override.End.String()
		}
		if override.LimitMinutes != nil {
			limit = strconv.FormatInt(*override.LimitMinutes, 10)
		}
		fmt.Fprintf(h, "%s=%s,%s,%s;", user, start, end, limit)
	}
	return int64(h.Sum64() >> 1)
}

// SetStatus stores the latest in-memory status for a hostname and user.
func (s *Store) SetStatus(hostname, user string, record StatusRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[statusKey{hostname: hostname, user: user}] = record
}

// StatusRecords returns all in-memory status records sorted by hostname and
// user.
func (s *Store) StatusRecords() []StatusRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]StatusRecord, 0, len(s.status))
	for _, record := range s.status {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Hostname != records[j].Hostname {
			return records[i].Hostname < records[j].Hostname
		}
		return records[i].User < records[j].User
	})
	return records
}

// PruneUsage removes usage records older than the given policy day.
func (s *Store) PruneUsage(beforeDay string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.usage {
		if key.day < beforeDay {
			delete(s.usage, key)
		}
	}
}

// Save writes usage to the configured CSV file. Status records stay in memory.
// A store with an empty path is memory-only and does nothing.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	records := make([]UsageRecord, 0, len(s.usage))
	for key, used := range s.usage {
		records = append(records, UsageRecord{
			Date:        key.day,
			User:        key.user,
			Hostname:    key.hostname,
			UsedSeconds: used,
		})
	}
	s.mu.Unlock()

	sort.Slice(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.Date != b.Date {
			return a.Date < b.Date
		}
		if a.User != b.User {
			return a.User < b.User
		}
		return a.Hostname < b.Hostname
	})

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(dir, ".timetower-usage-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	writer := csv.NewWriter(temp)
	if err := writer.Write([]string{"date", "user", "hostname", "used_seconds"}); err != nil {
		temp.Close()
		return err
	}
	for _, record := range records {
		if err := writer.Write([]string{
			record.Date,
			record.User,
			record.Hostname,
			strconv.FormatInt(record.UsedSeconds, 10),
		}); err != nil {
			temp.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, s.path)
}
