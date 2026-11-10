// Package enforcer runs the Enforcer-side Timetower client and enforcement loop.
package enforcer

import (
	"context"
	"fmt"
	"log"
	"sort"
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
	FetchSchedule(context.Context) (*timetower.Schedule, int64, error)
	SendStatus(context.Context, timetower.StatusRequest) (*timetower.StatusResponse, int64, error)
}

// Config configures an Agent.
type Config struct {
	Client    Client
	Sessions  session.SessionSource
	Processes *process.Cache
	Version   string
	Interval  time.Duration
	Now       func() time.Time
}

// Agent fetches its schedule from Timetower, enforces it locally, and reports
// status back to Timetower. It keeps no schedule or usage files.
type Agent struct {
	client    Client
	sessions  session.SessionSource
	processes *process.Cache
	version   string
	interval  time.Duration
	now       func() time.Time

	schedule       *timetower.Schedule
	loc            *time.Location
	revision       int64
	statusInterval time.Duration
	offlineGrace   time.Duration
	lastContact    time.Time
	lastTick       time.Time
	day            string

	subjects  map[string]*subjectState
	order     []string
	byAccount map[string]string

	lastSessions []session.Session
	lastAudited  map[string]session.Session
	auditReady   bool
	sessionError string
	reportError  string
}

type subjectState struct {
	account   string
	policy    *timetower.Policy
	local     int64
	reported  int64
	aggregate int64
	pending   map[string]int64
}

// New returns an Agent. The caller must provide a client and session source.
func New(config Config) *Agent {
	interval := config.Interval
	if interval <= 0 {
		interval = DefaultInterval
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Agent{
		client:         config.Client,
		sessions:       config.Sessions,
		processes:      config.Processes,
		version:        config.Version,
		interval:       interval,
		now:            now,
		loc:            time.UTC,
		statusInterval: DefaultStatusInterval,
		subjects:       make(map[string]*subjectState),
		byAccount:      make(map[string]string),
		lastAudited:    make(map[string]session.Session),
	}
}

// Run fetches the initial schedule, enforces it, and reports status until ctx
// is cancelled. A missing initial schedule is a startup failure.
func (a *Agent) Run(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("timetower client is nil")
	}
	if a.sessions == nil {
		return fmt.Errorf("session source is nil")
	}
	if err := a.fetchSchedule(ctx); err != nil {
		return fmt.Errorf("fetch schedule: %w", err)
	}

	a.enforce(ctx)
	if err := a.reportStatus(ctx); err != nil {
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
			if err := a.reportStatus(finalCtx); err != nil {
				a.logReportError(err)
			}
			cancel()
			return nil
		case <-enforceTicker.C:
			a.enforce(ctx)
		case <-statusTicker.C:
			before := a.statusInterval
			if err := a.reportStatus(ctx); err != nil {
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

func (a *Agent) fetchSchedule(ctx context.Context) error {
	schedule, revision, err := a.client.FetchSchedule(ctx)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", schedule.Timezone, err)
	}

	now := a.now().UTC()
	day := now.In(loc).Format(dateLayout)
	if !schedule.ServerTime.IsZero() {
		day = schedule.ServerTime.In(loc).Format(dateLayout)
	}
	a.applySchedule(schedule, revision, loc, day, now)
	log.Printf("schedule: revision %d, %d subject(s)", revision, len(schedule.Subjects))
	return nil
}

func (a *Agent) applySchedule(schedule *timetower.Schedule, revision int64, loc *time.Location, day string, now time.Time) {
	a.schedule = schedule
	a.loc = loc
	a.revision = revision
	a.statusInterval = time.Duration(schedule.StatusIntervalSeconds) * time.Second
	if a.statusInterval <= 0 {
		a.statusInterval = DefaultStatusInterval
	}
	a.offlineGrace = time.Duration(schedule.OfflineGraceSeconds) * time.Second
	if a.offlineGrace < 0 {
		a.offlineGrace = 0
	}

	a.rolloverDay(day)

	subjects := make(map[string]*subjectState, len(schedule.Subjects))
	order := make([]string, 0, len(schedule.Subjects))
	byAccount := make(map[string]string, len(schedule.Subjects))
	for _, item := range schedule.Subjects {
		state := a.subjects[item.Subject]
		if state == nil {
			state = &subjectState{pending: make(map[string]int64)}
		}
		if state.pending == nil {
			state.pending = make(map[string]int64)
		}
		state.account = item.Account
		state.policy = item.Policy
		state.aggregate = item.Usage.UsedSeconds
		subjects[item.Subject] = state
		order = append(order, item.Subject)
		byAccount[item.Account] = item.Subject
	}
	a.subjects = subjects
	a.order = order
	a.byAccount = byAccount
	a.lastContact = now
}

func (a *Agent) enforce(ctx context.Context) {
	now := a.now()
	a.rolloverDay(now.In(a.loc).Format(dateLayout))

	sessions, err := a.sessions.Sessions(ctx)
	if err != nil {
		a.setSessionError(err)
		return
	}
	a.clearSessionError()

	a.auditSessions(sessions)
	a.lastSessions = sessions

	elapsed := a.elapsed(now)
	for _, subject := range a.order {
		state := a.subjects[subject]
		if hasActiveSession(sessions, state.account) {
			state.local += int64(elapsed / time.Second)
		}
	}

	for _, sess := range sessions {
		if sess.ID == "" || !session.IsLocalSession(sess) {
			continue
		}
		subject, ok := a.byAccount[sess.User]
		if !ok {
			continue
		}
		if a.allowed(a.subjects[subject], now) {
			continue
		}
		log.Printf("AUDIT: terminate user=%q session=%s reason=schedule_denied", sess.User, sess.ID)
		if err := a.sessions.TerminateSession(ctx, sess.ID); err != nil {
			log.Printf("WARN: terminate session %s for user %s: %v", sess.ID, sess.User, err)
		}
	}
}

func (a *Agent) reportStatus(ctx context.Context) error {
	now := a.now().UTC()
	a.rolloverDay(now.In(a.loc).Format(dateLayout))

	request := timetower.StatusRequest{
		ObservedAt:      now,
		EnforcerVersion: a.version,
		Health:          a.health(),
		Subjects:        a.statusSubjects(),
		Processes:       a.processStats(),
	}
	sent := make(map[string]int64, len(a.order))
	for _, subject := range a.order {
		sent[subject] = a.subjects[subject].local
	}

	response, revision, err := a.client.SendStatus(ctx, request)
	if err != nil {
		return err
	}

	a.lastContact = now
	a.applyStatus(response, sent)
	if revision != a.revision {
		if err := a.fetchSchedule(ctx); err != nil {
			return fmt.Errorf("refresh schedule: %w", err)
		}
	}
	return nil
}

func (a *Agent) applyStatus(response *timetower.StatusResponse, sent map[string]int64) {
	dayChanged := response.ServerDay != "" && response.ServerDay != a.day
	if dayChanged {
		a.rolloverDay(response.ServerDay)
	}
	for _, item := range response.Subjects {
		state := a.subjects[item.Subject]
		if state == nil {
			continue
		}
		sentLocal := sent[item.Subject]
		if dayChanged {
			// The sent counter belongs to the previous policy day and was
			// moved into the pending set by the rollover.
			sentLocal = 0
		}
		if item.ReportedUsedSeconds > sentLocal {
			// The server already had a higher counter, for example after an
			// Enforcer restart. Keep the local usage counted since startup.
			state.local = item.ReportedUsedSeconds + sentLocal
		} else {
			state.local = sentLocal
		}
		state.reported = item.ReportedUsedSeconds
		state.aggregate = item.AggregateUsedSeconds
		for _, day := range item.AcknowledgedDays {
			if day != a.day {
				delete(state.pending, day)
			}
		}
	}
}

func (a *Agent) statusSubjects() []timetower.StatusSubject {
	subjects := make([]timetower.StatusSubject, 0, len(a.order))
	for _, subject := range a.order {
		state := a.subjects[subject]
		sessions := sessionsForAccount(a.lastSessions, state.account)
		active, locked := sessionState(sessions)

		usage := make([]timetower.UsageEntry, 0, len(state.pending)+1)
		usage = append(usage, timetower.UsageEntry{Day: a.day, UsedSeconds: state.local})
		for day, used := range state.pending {
			if day == a.day {
				continue
			}
			usage = append(usage, timetower.UsageEntry{Day: day, UsedSeconds: used})
		}
		sort.Slice(usage, func(i, j int) bool { return usage[i].Day < usage[j].Day })
		if len(usage) > 7 {
			usage = usage[len(usage)-7:]
		}

		subjects = append(subjects, timetower.StatusSubject{
			Subject:        subject,
			Account:        state.account,
			AccountPresent: true,
			SessionCount:   len(sessions),
			Active:         active,
			Locked:         locked,
			Usage:          usage,
		})
	}
	return subjects
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
			User:            entry.User,
			Name:            entry.Name,
			Scope:           scope,
			Instances:       entry.Count,
			MemoryRSSBytes:  int64(entry.MemoryBytes),
			CPUSecondsTotal: entry.CPUSeconds,
		})
	}
	return stats
}

func (a *Agent) rolloverDay(day string) {
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

	for _, state := range a.subjects {
		if state.local > state.pending[a.day] {
			state.pending[a.day] = state.local
		}
		state.local = 0
		state.reported = 0
		state.aggregate = 0
		delete(state.pending, day)
		prunePending(state.pending, day)
	}
	a.day = day
}

func (a *Agent) allowed(state *subjectState, now time.Time) bool {
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

func (a *Agent) estimated(state *subjectState) int64 {
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

func (a *Agent) health() timetower.Health {
	health := timetower.Health{Status: "ok"}
	if a.sessionError != "" {
		health.Status = "degraded"
		err := a.sessionError
		health.LastError = &err
	}
	return health
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
	for _, session := range sessions {
		if session.ID == "" {
			continue
		}
		if _, ok := a.byAccount[session.User]; !ok {
			continue
		}
		current[session.ID] = session
	}

	if !a.auditReady {
		a.lastAudited = current
		a.auditReady = true
		return
	}

	for id, session := range current {
		previous, ok := a.lastAudited[id]
		if !ok {
			logAudit("login", session)
			continue
		}
		if previous.Locked != session.Locked {
			if session.Locked {
				logAudit("lock", session)
			} else {
				logAudit("unlock", session)
			}
		}
	}
	for id, session := range a.lastAudited {
		if _, ok := current[id]; !ok {
			logAudit("logout", session)
		}
	}
	a.lastAudited = current
}

func logAudit(event string, s session.Session) {
	log.Printf("AUDIT: %s user=%q session=%s seat=%q remote=%t class=%q",
		event, s.User, s.ID, s.Seat, s.Remote, s.Class)
}

func hasActiveSession(sessions []session.Session, account string) bool {
	for _, sess := range sessions {
		if sess.User == account && session.IsCountedSession(sess) && sess.Active && !sess.Locked {
			return true
		}
	}
	return false
}

func sessionsForAccount(sessions []session.Session, account string) []session.Session {
	var out []session.Session
	for _, sess := range sessions {
		if sess.User == account {
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

func prunePending(pending map[string]int64, day string) {
	cutoffTime, err := time.Parse(dateLayout, day)
	if err != nil {
		return
	}
	cutoff := cutoffTime.AddDate(0, 0, -6).Format(dateLayout)
	for entry := range pending {
		if entry < cutoff {
			delete(pending, entry)
		}
	}
}
