package process

import (
	"sort"
	"sync"
)

// Entry is the cached state of one user and executable pair.
type Entry struct {
	// User is the name of the user that owns the processes.
	User string
	// Name is the short name of the executable.
	Name string
	// Executable is the path of the executable.
	Executable string
	// Count is the number of running processes with this executable.
	Count int
	// MemoryBytes is the total resident memory of the processes in bytes.
	MemoryBytes uint64
	// CPUSeconds is the CPU time that TimeLord observed for this executable
	// since it started. The value never decreases.
	CPUSeconds float64
}

// key identifies one cache entry.
type key struct {
	user       string
	executable string
}

// processState is the last CPU reading for one PID.
type processState struct {
	startTime  uint64
	cpuSeconds float64
}

// Cache is a concurrency-safe in-memory store of processes.
// TimeLord keys the cache by user and executable.
type Cache struct {
	mu      sync.RWMutex
	entries map[key]Entry
	prev    map[int]processState
}

// NewCache returns an empty process cache.
func NewCache() *Cache {
	return &Cache{
		entries: make(map[key]Entry),
		prev:    make(map[int]processState),
	}
}

// Set replaces the process list in the cache.
// It keeps the accumulated CPU time of each user and executable pair, so the
// CPU value only increases.
func (c *Cache) Set(processes []Process) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make(map[key]Entry, len(processes))
	prev := make(map[int]processState, len(processes))

	for _, p := range processes {
		k := key{user: p.User, executable: p.Executable}

		entry, ok := entries[k]
		if !ok {
			entry = Entry{
				User:       p.User,
				Name:       p.Name,
				Executable: p.Executable,
			}
			if old, found := c.entries[k]; found {
				entry.CPUSeconds = old.CPUSeconds
			}
		}

		entry.Count++
		entry.MemoryBytes += p.MemoryBytes

		if state, found := c.prev[p.PID]; found && state.startTime == p.StartTime {
			if delta := p.CPUSeconds - state.cpuSeconds; delta > 0 {
				entry.CPUSeconds += delta
			}
		}

		entries[k] = entry
		prev[p.PID] = processState{startTime: p.StartTime, cpuSeconds: p.CPUSeconds}
	}

	c.entries = entries
	c.prev = prev
}

// Get returns the entry for a user and executable path.
func (c *Cache) Get(user, executable string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key{user: user, executable: executable}]
	return entry, ok
}

// Snapshot returns a copy of the cache, sorted by user and executable path.
func (c *Cache) Snapshot() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]Entry, 0, len(c.entries))
	for _, entry := range c.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].User != out[j].User {
			return out[i].User < out[j].User
		}
		return out[i].Executable < out[j].Executable
	})
	return out
}
