package process

import (
	"sort"
	"sync"
)

// Entry is the cached state of one executable.
type Entry struct {
	Process
	// Count is the number of running processes with this executable.
	Count int
}

// Cache is a concurrency-safe in-memory store of processes, keyed by executable.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewCache returns an empty process cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]Entry)}
}

// Set replaces the cache with the given processes.
// Processes with the same executable become one entry with a count.
func (c *Cache) Set(processes []Process) {
	entries := make(map[string]Entry, len(processes))
	for _, p := range processes {
		entry, ok := entries[p.Executable]
		if !ok {
			entry = Entry{Process: p}
		}
		entry.Count++
		entries[p.Executable] = entry
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = entries
}

// Get returns the entry for an executable path.
func (c *Cache) Get(executable string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[executable]
	return entry, ok
}

// Snapshot returns a copy of the cache, sorted by executable path.
func (c *Cache) Snapshot() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]Entry, 0, len(c.entries))
	for _, entry := range c.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Executable < out[j].Executable
	})
	return out
}
