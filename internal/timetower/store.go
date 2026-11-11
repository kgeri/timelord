package timetower

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Store holds usage counters and the latest status per hostname and user.
type Store struct {
	mu     sync.Mutex
	path   string
	usage  map[usageKey]int64
	status map[statusKey]StatusRecord
}

// StatusRecord is the latest status received for one hostname and user.
type StatusRecord struct {
	Hostname   string        `json:"hostname"`
	User       string        `json:"user"`
	ReceivedAt time.Time     `json:"received_at"`
	Request    StatusRequest `json:"request"`
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

type persistedState struct {
	Usage  []UsageRecord  `json:"usage"`
	Status []StatusRecord `json:"status"`
}

// UsageRecord is one persisted usage counter.
type UsageRecord struct {
	Hostname    string `json:"hostname"`
	User        string `json:"user"`
	Day         string `json:"day"`
	UsedSeconds int64  `json:"used_seconds"`
}

// NewStore creates a store and loads the state file at path when it exists.
// An empty path keeps state in memory only.
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:   path,
		usage:  make(map[usageKey]int64),
		status: make(map[statusKey]StatusRecord),
	}
	if path == "" {
		return store, nil
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, record := range state.Usage {
		if record.Hostname == "" || record.User == "" || record.UsedSeconds < 0 {
			continue
		}
		key := usageKey{hostname: record.Hostname, user: record.User, day: record.Day}
		if record.UsedSeconds > s.usage[key] {
			s.usage[key] = record.UsedSeconds
		}
	}
	for _, record := range state.Status {
		if record.Hostname == "" || record.User == "" {
			continue
		}
		s.status[statusKey{hostname: record.Hostname, user: record.User}] = record
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

// SetStatus stores the latest status for a hostname and user.
func (s *Store) SetStatus(hostname, user string, record StatusRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[statusKey{hostname: hostname, user: user}] = record
}

// StatusRecords returns all stored status records sorted by hostname and user.
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

// Save writes the current state to the configured state file. A store with an
// empty path is memory-only and does nothing.
func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := persistedState{
		Usage:  make([]UsageRecord, 0, len(s.usage)),
		Status: make([]StatusRecord, 0, len(s.status)),
	}
	for key, used := range s.usage {
		state.Usage = append(state.Usage, UsageRecord{
			Hostname:    key.hostname,
			User:        key.user,
			Day:         key.day,
			UsedSeconds: used,
		})
	}
	sort.Slice(state.Usage, func(i, j int) bool {
		a, b := state.Usage[i], state.Usage[j]
		if a.Hostname != b.Hostname {
			return a.Hostname < b.Hostname
		}
		if a.User != b.User {
			return a.User < b.User
		}
		return a.Day < b.Day
	})
	for _, record := range s.status {
		state.Status = append(state.Status, record)
	}
	sort.Slice(state.Status, func(i, j int) bool {
		if state.Status[i].Hostname != state.Status[j].Hostname {
			return state.Status[i].Hostname < state.Status[j].Hostname
		}
		return state.Status[i].User < state.Status[j].User
	})

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

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

	if _, err := temp.Write(data); err != nil {
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
