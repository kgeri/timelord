// Package enforce ends user sessions when their schedule does not permit use.
package enforce

import (
	"context"
	"log"
	"sync"
	"time"

	"timelord/internal/schedule"
)

// DefaultInterval is the time between two session checks.
const DefaultInterval = 15 * time.Second

// Session is one login session as reported by the operating system.
type Session struct {
	ID      string
	User    string
	UID     string
	Seat    string
	Class   string
	Desktop string
	Remote  bool
	Active  bool
	Locked  bool
}

// SessionSource discovers sessions and terminates one session.
type SessionSource interface {
	Sessions(ctx context.Context) ([]Session, error)
	TerminateSession(ctx context.Context, id string) error
}

// Engine tracks active use and terminates sessions that the schedule denies.
type Engine struct {
	source   SessionSource
	store    *schedule.Store
	users    []string
	managed  map[string]bool
	interval time.Duration
	now      func() time.Time

	mu            sync.Mutex
	date          string
	used          map[string]time.Duration
	lastTick      time.Time
	sessionErrLog string
}

// NewEngine returns an engine for the users managed by store.
func NewEngine(source SessionSource, store *schedule.Store) *Engine {
	users := store.Users()
	managed := make(map[string]bool, len(users))
	for _, user := range users {
		managed[user] = true
	}

	return &Engine{
		source:   source,
		store:    store,
		users:    users,
		managed:  managed,
		interval: DefaultInterval,
		now:      time.Now,
		used:     make(map[string]time.Duration),
	}
}

// Run checks sessions until ctx is cancelled. It checks immediately, then waits
// for the next interval.
func (e *Engine) Run(ctx context.Context) {
	e.tick(ctx)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

// UsageSnapshot returns the current daily usage of every managed user.
func (e *Engine) UsageSnapshot() []schedule.Usage {
	now := e.now()

	e.mu.Lock()
	defer e.mu.Unlock()
	e.rolloverLocked(now)

	out := make([]schedule.Usage, 0, len(e.users))
	for _, user := range e.users {
		policy, ok := e.store.Policy(user)
		var limit time.Duration
		if ok {
			limit = time.Duration(policy.DailyLimitMinutes) * time.Minute
		}
		out = append(out, schedule.Usage{
			User:      user,
			Used:      e.used[user],
			Limit:     limit,
			HasPolicy: ok,
		})
	}
	return out
}

// tick reads sessions, accumulates active time, and terminates denied sessions.
func (e *Engine) tick(ctx context.Context) {
	now := e.now()

	sessions, err := e.source.Sessions(ctx)
	if err != nil {
		e.logSessionError(err)
		return
	}
	e.clearSessionError()

	e.recordUsage(now, sessions)
	e.terminateDenied(ctx, now, sessions)
}

// recordUsage adds the time since the previous check for each user with an
// active, unlocked session.
func (e *Engine) recordUsage(now time.Time, sessions []Session) {
	active := make(map[string]bool)
	for _, session := range sessions {
		if e.managed[session.User] && isCountedSession(session) && session.Active && !session.Locked {
			active[session.User] = true
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.rolloverLocked(now)

	var elapsed time.Duration
	if !e.lastTick.IsZero() {
		elapsed = now.Sub(e.lastTick)
		if elapsed < 0 {
			elapsed = 0
		}
		// Do not charge a long suspend or a stalled process to the user.
		if elapsed > 2*e.interval {
			elapsed = e.interval
		}
	}
	e.lastTick = now

	for user := range active {
		e.used[user] += elapsed
	}
}

// terminateDenied ends local sessions when the current schedule denies access.
// This includes locked sessions, so the user cannot unlock into a denied
// period. Remote sessions such as SSH are left alone.
func (e *Engine) terminateDenied(ctx context.Context, now time.Time, sessions []Session) {
	for _, session := range sessions {
		if !e.managed[session.User] || !isLocalSession(session) || session.ID == "" {
			continue
		}
		if e.store.Allowed(session.User, now, e.usedFor(session.User)) {
			continue
		}
		log.Printf("schedule: terminating session %s for user %s", session.ID, session.User)
		if err := e.source.TerminateSession(ctx, session.ID); err != nil {
			log.Printf("WARN: terminate session %s for user %s: %v", session.ID, session.User, err)
		}
	}
}

// usedFor returns the active time counted for user today.
func (e *Engine) usedFor(user string) time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.used[user]
}

// rolloverLocked resets usage when the local date changes.
func (e *Engine) rolloverLocked(now time.Time) {
	date := now.In(time.Local).Format("2006-01-02")
	if e.date == date {
		return
	}
	e.date = date
	e.used = make(map[string]time.Duration)
}

func (e *Engine) logSessionError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sessionErrLog == err.Error() {
		return
	}
	e.sessionErrLog = err.Error()
	log.Printf("WARN: session discovery failed: %v", err)
}

func (e *Engine) clearSessionError() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sessionErrLog == "" {
		return
	}
	e.sessionErrLog = ""
	log.Println("session discovery recovered")
}

// isLocalSession reports whether the session is on a local seat. SSH and other
// remote sessions are not PC use.
func isLocalSession(session Session) bool {
	return session.Seat != "" && !session.Remote
}

// isCountedSession reports whether the session contributes to active use. Lock
// screens can be local sessions but must not add time.
func isCountedSession(session Session) bool {
	if !isLocalSession(session) {
		return false
	}
	return session.Class == "" || session.Class == "user"
}
