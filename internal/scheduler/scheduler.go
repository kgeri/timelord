package scheduler

import (
	"context"
	"log"
	"time"

	"timelord/internal/process"
)

// DefaultInterval is the time between two process checks.
const DefaultInterval = time.Minute

// Scheduler checks the process list at a fixed interval.
type Scheduler struct {
	lister   process.Lister
	cache    *process.Cache
	interval time.Duration
}

// New returns a Scheduler that reads processes from lister and stores them in cache.
func New(lister process.Lister, cache *process.Cache) *Scheduler {
	return &Scheduler{
		lister:   lister,
		cache:    cache,
		interval: DefaultInterval,
	}
}

// Run checks the process list until ctx is cancelled.
// It checks the list immediately, then waits for the next interval.
func (s *Scheduler) Run(ctx context.Context) {
	s.update()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.update()
		}
	}
}

// SetInterval changes the time between two process checks.
func (s *Scheduler) SetInterval(interval time.Duration) {
	s.interval = interval
}

// update refreshes the cache and prints the process list.
func (s *Scheduler) update() {
	processes, err := s.lister.List()
	if err != nil {
		log.Printf("failed to list processes: %v", err)
		return
	}

	s.cache.Set(processes)
	s.print()
}

// print writes the cached process list to the log.
func (s *Scheduler) print() {
	for _, e := range s.cache.Snapshot() {
		log.Printf("user=%q name=%q executable=%q count=%d", e.User, e.Name, e.Executable, e.Count)
	}
}
