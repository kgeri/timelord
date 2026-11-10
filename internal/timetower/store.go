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

// Store holds usage counters and the latest status per Enforcer.
type Store struct {
	mu     sync.Mutex
	path   string
	usage  map[usageKey]int64
	status map[string]StatusRecord
}

// StatusRecord is the latest status received from an Enforcer.
type StatusRecord struct {
	Enforcer   string        `json:"enforcer"`
	ReceivedAt time.Time     `json:"received_at"`
	Request    StatusRequest `json:"request"`
}

type usageKey struct {
	enforcer string
	subject  string
	day      string
}

type persistedState struct {
	Usage  []UsageRecord  `json:"usage"`
	Status []StatusRecord `json:"status"`
}

// UsageRecord is one persisted usage counter.
type UsageRecord struct {
	Enforcer    string `json:"enforcer"`
	Subject     string `json:"subject"`
	Day         string `json:"day"`
	UsedSeconds int64  `json:"used_seconds"`
}

// NewStore creates a store and loads the state file at path when it exists.
// An empty path keeps state in memory only.
func NewStore(path string) (*Store, error) {
	store := &Store{
		path:   path,
		usage:  make(map[usageKey]int64),
		status: make(map[string]StatusRecord),
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
		if record.UsedSeconds < 0 {
			continue
		}
		key := usageKey{enforcer: record.Enforcer, subject: record.Subject, day: record.Day}
		if record.UsedSeconds > s.usage[key] {
			s.usage[key] = record.UsedSeconds
		}
	}
	for _, record := range state.Status {
		if record.Enforcer != "" {
			s.status[record.Enforcer] = record
		}
	}
	return nil
}

// RecordUsage stores used seconds for one Enforcer, subject, and day. The
// counter is monotonic; a lower value never replaces a higher one.
func (s *Store) RecordUsage(enforcer, subject, day string, usedSeconds int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := usageKey{enforcer: enforcer, subject: subject, day: day}
	if usedSeconds > s.usage[key] {
		s.usage[key] = usedSeconds
	}
}

// Usage returns the stored counter for one Enforcer, subject, and day.
func (s *Store) Usage(enforcer, subject, day string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage[usageKey{enforcer: enforcer, subject: subject, day: day}]
}

// AggregateUsage returns the sum of all Enforcers' counters for a subject/day.
func (s *Store) AggregateUsage(subject, day string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for key, used := range s.usage {
		if key.subject == subject && key.day == day {
			total += used
		}
	}
	return total
}

// SetStatus stores the latest status for an Enforcer.
func (s *Store) SetStatus(enforcer string, record StatusRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[enforcer] = record
}

// Status returns the latest status for an Enforcer.
func (s *Store) Status(enforcer string) (StatusRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.status[enforcer]
	return record, ok
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
			Enforcer:    key.enforcer,
			Subject:     key.subject,
			Day:         key.day,
			UsedSeconds: used,
		})
	}
	sort.Slice(state.Usage, func(i, j int) bool {
		a, b := state.Usage[i], state.Usage[j]
		if a.Enforcer != b.Enforcer {
			return a.Enforcer < b.Enforcer
		}
		if a.Subject != b.Subject {
			return a.Subject < b.Subject
		}
		return a.Day < b.Day
	})
	for _, record := range s.status {
		state.Status = append(state.Status, record)
	}
	sort.Slice(state.Status, func(i, j int) bool {
		return state.Status[i].Enforcer < state.Status[j].Enforcer
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
