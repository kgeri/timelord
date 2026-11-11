// Package enforcer runs the Enforcer-side Timetower client and enforcement loop.
package enforcer

import (
	"context"
	"fmt"
	"log"
	"time"

	"timelord/internal/process"
	"timelord/internal/session"
	"timelord/internal/timetower"
)

const (
	// DefaultInterval is the time between session checks.
	DefaultInterval = 15 * time.Second
	// DefaultStatusInterval is used when the schedule omits a valid interval.
	DefaultStatusInterval = 60 * time.Second

	dateLayout = "2006-01-02"
)

// Client is the subset of the Timetower client used by the agent.
type Client interface {
	FetchSchedule(context.Context, string) (*timetower.Schedule, int64, error)
	SendStatus(context.Context, string, timetower.StatusRequest) (*timetower.StatusResponse, int64, error)
}

// Config configures an Agent.
type Config struct {
	Client    Client
	Sessions  session.SessionSource
	Processes *process.Cache
	Version   string
	Hostname  string
	Users     []string
	Interval  time.Duration
	Now       func() time.Time
}

// Agent fetches schedules from Timetower, enforces them locally, and reports
// status back to Timetower. It keeps no schedule or usage files.
type Agent struct {
	client    Client
	sessions  session.SessionSource
	processes *process.Cache
	version   string
	hostname  string
	users     []string
	managed   map[string]bool
	interval  time.Duration
	now       func() time.Time

	loc            *time.Location
	revision       int64
	statusInterval time.Duration
	offlineGrace   time.Duration
	lastContact    time.Time
	lastTick       time.Time
	day            string

	states map[string]*userState

	lastSessions []session.Session
	lastAudited  map[string]session.Session
	auditReady   bool
	sessionError string
	reportError  string
}

type userState struct {
	policy    *timetower.Policy
	local     int64
	reported  int64
	aggregate int64
}

// New returns an Agent. The caller must provide a client, session source, and
// at least one user.
func New(config Config) *Agent {
	interval := config.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	users := make([]string, 0, len(config.Users))
	managed := make(map[string]bool, len(config.Users))
	states := make(map[string]*userState, len(config.Users))
	for _, user := range config.Users {
		if user == "" || managed[user] {
			continue
		}
		managed[user] = true
		users = append(users, user)
		states[user] = &userState{}
	}

	return &Agent{
		client:         config.Client,
		sessions:       config.Sessions,
		processes:      config.Processes,
		version:        config.Version,
		hostname:       config.Hostname,
		users:          users,
		managed:        managed,
		interval:       interval,
		now:            now,
		loc:            time.UTC,
		statusInterval: DefaultStatusInterval,
		states:         states,
		lastAudited:    make(map[string]session.Session),
	}
}

// Run fetches the initial schedules, enforces them, and reports status until
// ctx is cancelled. A missing initial schedule is a startup failure.
func (a *Agent) Run(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("timetower client is nil")
	}
	if a.sessions == nil {
		return fmt.Errorf("session source is nil")
	}
	if a.hostname == "" {
		return fmt.Errorf("hostname is empty")
	}
	if len(a.users) == 0 {
		return fmt.Errorf("no users configured")
	}
	if err := a.fetchSchedules(ctx); err != nil {
		return fmt.Errorf("fetch schedule: %w", err)
	}

	a.enforce(ctx)
	if err := a.reportStatuses(ctx); err != nil {
		a.logReportError(err)
	} else {
		a.clearReportError()
	}

	enforceTicker := time.NewTicker(a.interval)
	defer enforceTicker.Stop()
	statusTicker := time.NewTicker(a.statusInterval)
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			finalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := a.reportStatuses(finalCtx); err != nil {
				a.logReportError(err)
			}
			cancel()
			return nil
		case <-enforceTicker.C:
			a.enforce(ctx)
		case <-statusTicker.C:
			before := a.statusInterval
			if err := a.reportStatuses(ctx); err != nil {
				a.logReportError(err)
			} else {
				a.clearReportError()
			}
			if a.statusInterval != before {
				statusTicker.Stop()
				statusTicker = time.NewTicker(a.statusInterval)
			}
		}
	}
}

func (a *Agent) fetchSchedules(ctx context.Context) error {
	if len(a.users) == 0 {
		return fmt.Errorf("no users configured")
	}
	now := a.now().UTC()

	type fetchedSchedule struct {
		user     string
		schedule *timetower.Schedule
		revision int64
	}
	fetched := make([]fetchedSchedule, 0, len(a.users))
	for _, user := range a.users {
		schedule, revision, err := a.client.FetchSchedule(ctx, user)
		if err != nil {
			return fmt.Errorf("user %s: %w", user, err)
		}
		fetched = append(fetched, fetchedSchedule{user: user, schedule: schedule, revision: revision})
	}

	first := fetched[0].schedule
	loc, err := time.LoadLocation(first.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", first.Timezone, err)
	}
	day := now.In(loc).Format(dateLayout)
	if !first.ServerTime.IsZero() {
		day = first.ServerTime.In(loc).Format(dateLayout)
	}
	statusInterval := time.Duration(first.StatusIntervalSeconds) * time.Second
	if statusInterval <= 0 {
		statusInterval = DefaultStatusInterval
	}
	offlineGrace := time.Duration(first.OfflineGraceSeconds) * time.Second
	if offlineGrace < 0 {
		offlineGrace = 0
	}

	a.loc = loc
	a.statusInterval = statusInterval
	a.offlineGrace = offlineGrace
	a.revision = fetched[0].revision
	a.setDay(day)
	for _, item := range fetched {
		state := a.states[item.user]
		if state == nil {
			state = &userState{}
			a.states[item.user] = state
		}
		state.policy = item.schedule.Policy
		state.aggregate = item.schedule.Usage.UsedSeconds
	}
	a.lastContact = now

	log.Printf("schedule: revision %d, %d user(s)", a.revision, len(a.users))
	return nil
}

func (a *Agent) enforce(ctx context.Context) {
	now := a.now()
	a.setDay(now.In(a.loc).Format(dateLayout))

	sessions, err := a.sessions.Sessions(ctx)
	if err != nil {
		a.setSessionError(err)
		return
	}
	a.clearSessionError()

	a.auditSessions(sessions)
	a.lastSessions = sessions

	elapsed := a.elapsed(now)
	for _, user := range a.users {
		if hasActiveSession(sessions, user) {
			a.states[user].local += int64(elapsed / time.Second)
		}
	}

	for _, sess := range sessions {
		if sess.ID == "" || !session.IsLocalSession(sess) || !a.managed[sess.User] {
			continue
		}
		state := a.states[sess.User]
		if a.allowed(state, now) {
			continue
		}
		log.Printf("AUDIT: terminate user=%q session=%s reason=schedule_denied", sess.User, sess.ID)
		if err := a.sessions.TerminateSession(ctx, sess.ID); err != nil {
			log.Printf("WARN: terminate session %s for user %s: %v", sess.ID, sess.User, err)
		}
	}
}

func (a *Agent) reportStatuses(ctx context.Context) error {
	now := a.now().UTC()
	a.setDay(now.In(a.loc).Format(dateLayout))
	processes := a.processStats()

	var firstErr error
	refresh := false

	for _, user := range a.users {
		state := a.states[user]
		request := a.statusRequest(user, state, now, processes)
		sentLocal := state.local

		response, revision, err := a.client.SendStatus(ctx, user, request)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("user %s: %w", user, err)
			}
			continue
		}

		a.lastContact = now
		a.applyStatus(user, state, response, sentLocal)
		if revision != a.revision {
			refresh = true
		}
	}

	if refresh {
		if err := a.fetchSchedules(ctx); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("refresh schedules: %w", err)
			}
		}
	}
	return firstErr
}

func (a *Agent) statusRequest(user string, state *userState, now time.Time, processes []timetower.ProcessStat) timetower.StatusRequest {
	sessions := sessionsForUser(a.lastSessions, user)
	active, locked := sessionState(sessions)

	return timetower.StatusRequest{
		ObservedAt:      now,
		EnforcerVersion: a.version,
		Hostname:        a.hostname,
		Active:          active,
		Locked:          locked,
		SessionCount:    len(sessions),
		UsageSeconds:    state.local,
		Processes:       processes,
	}
}

func (a *Agent) applyStatus(user string, state *userState, response *timetower.StatusResponse, sentLocal int64) {
	if response.ServerDay != "" && response.ServerDay != a.day {
		a.setDay(response.ServerDay)
		sentLocal = 0
		state = a.states[user]
		if state == nil {
			return
		}
	}

	if response.ReportedUsedSeconds > sentLocal {
		// The server already had a higher counter, for example after an
		// Enforcer restart. Keep the local usage counted since startup.
		state.local = response.ReportedUsedSeconds + sentLocal
	} else {
		state.local = sentLocal
	}
	state.reported = response.ReportedUsedSeconds
	state.aggregate = response.AggregateUsedSeconds
}

func (a *Agent) processStats() []timetower.ProcessStat {
	if a.processes == nil {
		return []timetower.ProcessStat{}
	}
	entries := a.processes.Snapshot()
	stats := make([]timetower.ProcessStat, 0, len(entries))
	for _, entry := range entries {
		scope := entry.Scope.String()
		if scope != "app" {
			scope = "system"
		}
		stats = append(stats, timetower.ProcessStat{
			Name:            entry.Name,
			Scope:           scope,
			Instances:       entry.Count,
			MemoryRSSBytes:  int64(entry.MemoryBytes),
			CPUSecondsTotal: entry.CPUSeconds,
		})
	}
	return stats
}

func (a *Agent) setDay(day string) {
	if day == "" {
		return
	}
	if a.day == "" {
		a.day = day
		return
	}
	if a.day == day {
		return
	}

	for _, state := range a.states {
		state.local = 0
		state.reported = 0
		state.aggregate = 0
	}
	a.day = day
}

func (a *Agent) allowed(state *userState, now time.Time) bool {
	if state == nil {
		return false
	}
	if a.offlineGrace > 0 && !a.lastContact.IsZero() && now.Sub(a.lastContact) > a.offlineGrace {
		return false
	}
	if state.policy == nil || !state.policy.Enabled {
		return false
	}
	if !state.policy.InsideWindow(now, a.loc) {
		return false
	}
	limit := state.policy.DailyLimitSeconds
	return limit == 0 || a.estimated(state) < limit
}

func (a *Agent) estimated(state *userState) int64 {
	delta := state.local - state.reported
	if delta < 0 {
		delta = 0
	}
	return state.aggregate + delta
}

func (a *Agent) elapsed(now time.Time) time.Duration {
	if a.lastTick.IsZero() {
		a.lastTick = now
		return 0
	}
	elapsed := now.Sub(a.lastTick)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > 2*a.interval {
		elapsed = a.interval
	}
	a.lastTick = now
	return elapsed
}

func (a *Agent) setSessionError(err error) {
	message := err.Error()
	if message == a.sessionError {
		return
	}
	a.sessionError = message
	log.Printf("WARN: session discovery failed: %v", err)
}

func (a *Agent) clearSessionError() {
	if a.sessionError == "" {
		return
	}
	a.sessionError = ""
	log.Println("session discovery recovered")
}

func (a *Agent) logReportError(err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if message == a.reportError {
		return
	}
	a.reportError = message
	log.Printf("WARN: timetower status: %v", err)
}

func (a *Agent) clearReportError() {
	if a.reportError == "" {
		return
	}
	a.reportError = ""
	log.Println("timetower status recovered")
}

func (a *Agent) auditSessions(sessions []session.Session) {
	current := make(map[string]session.Session)
	for _, sess := range sessions {
		if sess.ID == "" || !a.managed[sess.User] {
			continue
		}
		current[sess.ID] = sess
	}

	if !a.auditReady {
		a.lastAudited = current
		a.auditReady = true
		return
	}

	for id, sess := range current {
		previous, ok := a.lastAudited[id]
		if !ok {
			logAudit("login", sess)
			continue
		}
		if previous.Locked != sess.Locked {
			if sess.Locked {
				logAudit("lock", sess)
			} else {
				logAudit("unlock", sess)
			}
		}
	}
	for id, sess := range a.lastAudited {
		if _, ok := current[id]; !ok {
			logAudit("logout", sess)
		}
	}
	a.lastAudited = current
}

func logAudit(event string, s session.Session) {
	log.Printf("AUDIT: %s user=%q session=%s seat=%q remote=%t class=%q",
		event, s.User, s.ID, s.Seat, s.Remote, s.Class)
}

func hasActiveSession(sessions []session.Session, user string) bool {
	for _, sess := range sessions {
		if sess.User == user && session.IsCountedSession(sess) && sess.Active && !sess.Locked {
			return true
		}
	}
	return false
}

func sessionsForUser(sessions []session.Session, user string) []session.Session {
	var out []session.Session
	for _, sess := range sessions {
		if sess.User == user {
			out = append(out, sess)
		}
	}
	return out
}

func sessionState(sessions []session.Session) (active, locked bool) {
	counted := 0
	allLocked := true
	for _, sess := range sessions {
		if !session.IsCountedSession(sess) {
			continue
		}
		counted++
		if !sess.Locked {
			allLocked = false
		}
		if sess.Active && !sess.Locked {
			active = true
		}
	}
	if counted == 0 {
		return false, false
	}
	return active, allLocked
}
