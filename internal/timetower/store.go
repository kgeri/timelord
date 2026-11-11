package timetower

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store holds usage counters and the latest in-memory status per hostname and
// user. Only usage is persisted.
type Store struct {
	mu     sync.Mutex
	saveMu sync.Mutex
	path   string
	usage  map[usageKey]int64
	status map[statusKey]StatusRecord
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

// UsageRecord is one row in the usage CSV.
type UsageRecord struct {
	Date        string
	User        string
	Hostname    string
	UsedSeconds int64
}

// NewStore creates a store and loads usage from the CSV at path. When the CSV
// does not exist, usage is migrated once from a sibling state.json file if one
// is present. An empty path keeps usage in memory only.
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:   path,
		usage:  make(map[usageKey]int64),
		status: make(map[statusKey]StatusRecord),
	}
	if path == "" {
		return store, nil
	}

	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		legacyPath := filepath.Join(filepath.Dir(path), "state.json")
		if err := store.loadLegacy(legacyPath); err != nil {
			log.Printf("WARN: ignore legacy state %s: %v", legacyPath, err)
		}
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

func (s *Store) loadLegacy(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var legacy struct {
		Usage []struct {
			Hostname    string `json:"hostname"`
			User        string `json:"user"`
			Day         string `json:"day"`
			UsedSeconds int64  `json:"used_seconds"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	for _, record := range legacy.Usage {
		if record.Hostname == "" || record.User == "" || record.UsedSeconds < 0 {
			continue
		}
		if _, err := time.Parse(dateLayout, record.Day); err != nil {
			continue
		}
		key := usageKey{hostname: record.Hostname, user: record.User, day: record.Day}
		if record.UsedSeconds > s.usage[key] {
			s.usage[key] = record.UsedSeconds
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
