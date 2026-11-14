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

// Store holds usage counters, current-day overrides, and the latest in-memory
// status per hostname and user. Usage and overrides are persisted.
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

type stateRecord struct {
	date         string
	user         string
	hostname     string
	usedSeconds  string
	start        string
	end          string
	limitMinutes string
}

// NewStore creates a store and loads usage and overrides from the CSV at path.
// A missing CSV starts empty. An empty path keeps everything in memory only.
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
		switch len(row) {
		case 4:
			if err := s.loadUsageRow(i+1, row); err != nil {
				return err
			}
		case 7:
			if strings.TrimSpace(row[2]) != "" {
				if strings.TrimSpace(row[4]) != "" || strings.TrimSpace(row[5]) != "" || strings.TrimSpace(row[6]) != "" {
					return fmt.Errorf("row %d: usage record has override fields", i+1)
				}
				if err := s.loadUsageRow(i+1, row[:4]); err != nil {
					return err
				}
				continue
			}
			if strings.TrimSpace(row[3]) != "" {
				return fmt.Errorf("row %d: override record has used_seconds", i+1)
			}
			if err := s.loadOverrideRow(i+1, row); err != nil {
				return err
			}
		default:
			return fmt.Errorf("row %d: expected 4 or 7 columns", i+1)
		}
	}
	return nil
}

func (s *Store) loadUsageRow(line int, row []string) error {
	date := strings.TrimSpace(row[0])
	user := strings.TrimSpace(row[1])
	hostname := strings.TrimSpace(row[2])
	used, err := strconv.ParseInt(strings.TrimSpace(row[3]), 10, 64)
	if err != nil || used < 0 || user == "" || hostname == "" {
		return fmt.Errorf("row %d: invalid usage record", line)
	}
	if _, err := time.Parse(dateLayout, date); err != nil {
		return fmt.Errorf("row %d: invalid date %q", line, date)
	}
	key := usageKey{hostname: hostname, user: user, day: date}
	if used > s.usage[key] {
		s.usage[key] = used
	}
	return nil
}

func (s *Store) loadOverrideRow(line int, row []string) error {
	date := strings.TrimSpace(row[0])
	user := strings.TrimSpace(row[1])
	if user == "" {
		return fmt.Errorf("row %d: invalid override record", line)
	}
	if _, err := time.Parse(dateLayout, date); err != nil {
		return fmt.Errorf("row %d: invalid date %q", line, date)
	}

	var override DayOverride
	if value := strings.TrimSpace(row[4]); value != "" {
		start, err := policy.ParseClock(value)
		if err != nil {
			return fmt.Errorf("row %d: invalid start: %w", line, err)
		}
		override.Start = &start
	}
	if value := strings.TrimSpace(row[5]); value != "" {
		end, err := policy.ParseClock(value)
		if err != nil {
			return fmt.Errorf("row %d: invalid end: %w", line, err)
		}
		override.End = &end
	}
	if value := strings.TrimSpace(row[6]); value != "" {
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit < 0 || limit > maxLimitMinutes {
			return fmt.Errorf("row %d: invalid limit_minutes", line)
		}
		override.LimitMinutes = &limit
	}
	if override.empty() {
		return fmt.Errorf("row %d: empty override record", line)
	}
	s.override[overrideKey{user: user, day: date}] = override
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

// AggregateUsageRange returns the per-day sums of all hostnames' counters for
// a user between two inclusive policy days. Days without usage are omitted.
func (s *Store) AggregateUsageRange(user, fromDay, toDay string) map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	totals := make(map[string]int64)
	for key, used := range s.usage {
		if key.user == user && key.day >= fromDay && key.day <= toDay {
			totals[key.day] += used
		}
	}
	return totals
}

// SetOverride stores the current-day override for one user and day. Older
// overrides for the same user are removed. Call Save to persist the change.
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

// Prune removes usage and override records older than the given policy day.
func (s *Store) Prune(beforeDay string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key := range s.usage {
		if key.day < beforeDay {
			delete(s.usage, key)
		}
	}
	for key := range s.override {
		if key.day < beforeDay {
			delete(s.override, key)
		}
	}
}

// Save writes usage and overrides to the configured state CSV. Status records
// stay in memory. A store with an empty path is memory-only and does nothing.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	records := make([]stateRecord, 0, len(s.usage)+len(s.override))
	for key, used := range s.usage {
		records = append(records, stateRecord{
			date:        key.day,
			user:        key.user,
			hostname:    key.hostname,
			usedSeconds: strconv.FormatInt(used, 10),
		})
	}
	for key, override := range s.override {
		record := stateRecord{date: key.day, user: key.user}
		if override.Start != nil {
			record.start = override.Start.String()
		}
		if override.End != nil {
			record.end = override.End.String()
		}
		if override.LimitMinutes != nil {
			record.limitMinutes = strconv.FormatInt(*override.LimitMinutes, 10)
		}
		records = append(records, record)
	}
	s.mu.Unlock()

	sort.Slice(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.date != b.date {
			return a.date < b.date
		}
		if a.user != b.user {
			return a.user < b.user
		}
		if a.hostname != b.hostname {
			return a.hostname < b.hostname
		}
		if a.start != b.start {
			return a.start < b.start
		}
		if a.end != b.end {
			return a.end < b.end
		}
		return a.limitMinutes < b.limitMinutes
	})

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	temp, err := os.CreateTemp(dir, ".timetower-state-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	writer := csv.NewWriter(temp)
	if err := writer.Write([]string{"date", "user", "hostname", "used_seconds", "start", "end", "limit_minutes"}); err != nil {
		temp.Close()
		return err
	}
	for _, record := range records {
		if err := writer.Write([]string{
			record.date,
			record.user,
			record.hostname,
			record.usedSeconds,
			record.start,
			record.end,
			record.limitMinutes,
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
