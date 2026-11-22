// Package enforcer runs the Enforcer-side Timetower client and enforcement loop.
package enforcer

import (
	"context"
	"fmt"
	"log"
	"time"

	"timelord/internal/notify"
	"timelord/internal/policy"
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

// Notice thresholds in minutes, sorted from the most urgent. A threshold is
// sent once per user, day, and limit; a zero means the limit or window is up.
var (
	limitNoticeMinutes  = []int{0, 5, 15}
	windowNoticeMinutes = []int{5, 10}
)

// weekdayDays maps Go weekdays to schedule days.
var weekdayDays = [...]policy.Day{
	policy.Sunday,
	policy.Monday,
	policy.Tuesday,
	policy.Wednesday,
	policy.Thursday,
	policy.Friday,
	policy.Saturday,
}

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
	Notifier  notify.Notifier
	Version   string
	Hostname  string
	Users     []User
	Interval  time.Duration
	Now       func() time.Time
}

// Agent fetches schedules from Timetower, enforces them locally, and reports
// status back to Timetower. It keeps no schedule or usage files.
type Agent struct {
	client      Client
	sessions    session.SessionSource
	processes   *process.Cache
	notifier    notify.Notifier
	version     string
	hostname    string
	users       []User
	serverUsers []string
	interval    time.Duration
	now         func() time.Time

	loc            *time.Location
	revision       int64
	statusInterval time.Duration
	offlineGrace   time.Duration
	lastContact    time.Time
	lastTick       time.Time
	day            string

	states   map[string]*userState
	notified map[string]bool

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

	users := make([]User, 0, len(config.Users))
	serverUsers := make([]string, 0, len(config.Users))
	serverSeen := make(map[string]bool, len(config.Users))
	states := make(map[string]*userState, len(config.Users))
	for _, user := range config.Users {
		if user.Local == "" || user.Server == "" || hasLocalUser(users, user.Local) {
			continue
		}
		users = append(users, user)
		if !serverSeen[user.Server] {
			serverSeen[user.Server] = true
			serverUsers = append(serverUsers, user.Server)
			states[user.Server] = &userState{}
		}
	}

	return &Agent{
		client:         config.Client,
		sessions:       config.Sessions,
		processes:      config.Processes,
		notifier:       config.Notifier,
		version:        config.Version,
		hostname:       config.Hostname,
		users:          users,
		serverUsers:    serverUsers,
		interval:       interval,
		now:            now,
		loc:            time.UTC,
		statusInterval: DefaultStatusInterval,
		states:         states,
		notified:       make(map[string]bool),
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
	if len(a.serverUsers) == 0 {
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

// serverForLocal returns the Timetower account for a local OS account. Session
// names are compared with the platform-specific case rules.
func (a *Agent) serverForLocal(local string) (string, bool) {
	for _, user := range a.users {
		if session.SameUser(user.Local, local) {
			return user.Server, true
		}
	}
	return "", false
}

// stateFor returns the Timetower state for a local session account.
func (a *Agent) stateFor(local string) *userState {
	server, ok := a.serverForLocal(local)
	if !ok {
		return nil
	}
	return a.states[server]
}

// isManaged reports whether a local account is mapped to Timetower.
func (a *Agent) isManaged(local string) bool {
	_, ok := a.serverForLocal(local)
	return ok
}

func hasLocalUser(users []User, local string) bool {
	for _, user := range users {
		if session.SameUser(user.Local, local) {
			return true
		}
	}
	return false
}

func (a *Agent) fetchSchedules(ctx context.Context) error {
	if len(a.serverUsers) == 0 {
		return fmt.Errorf("no users configured")
	}
	now := a.now().UTC()

	type fetchedSchedule struct {
		user     string
		schedule *timetower.Schedule
		revision int64
	}
	fetched := make([]fetchedSchedule, 0, len(a.serverUsers))
	for _, user := range a.serverUsers {
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

	log.Printf("schedule: revision %d, %d account(s)", a.revision, len(a.serverUsers))
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
	for _, user := range a.serverUsers {
		if a.hasActiveSession(sessions, user) {
			a.states[user].local += int64(elapsed / time.Second)
		}
	}

	a.notifyDue(ctx, sessions, now)

	for _, sess := range sessions {
		if sess.ID == "" || !session.IsCountedSession(sess) || !sess.Active || sess.Locked || !a.isManaged(sess.User) {
			continue
		}
		state := a.stateFor(sess.User)
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

	for _, user := range a.serverUsers {
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
	sessions := a.sessionsForServer(a.lastSessions, user)
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
	a.notified = make(map[string]bool)
	a.day = day
}

// notifyDue sends the next due reminder to one live session per user. Locked,
// inactive, remote, and unmanaged sessions are skipped.
func (a *Agent) notifyDue(ctx context.Context, sessions []session.Session, now time.Time) {
	if a.notifier == nil {
		return
	}
	// A session denied by the offline grace is logged off for connectivity, not
	// for running out of time, so do not send a misleading reminder.
	if a.offlineGrace > 0 && !a.lastContact.IsZero() && now.Sub(a.lastContact) > a.offlineGrace {
		return
	}

	seen := make(map[string]bool, len(a.serverUsers))
	for _, sess := range sessions {
		if sess.ID == "" || !session.IsCountedSession(sess) || !sess.Active || sess.Locked {
			continue
		}
		server, ok := a.serverForLocal(sess.User)
		if !ok || seen[server] {
			continue
		}
		state := a.states[server]
		if state == nil || state.policy == nil || !state.policy.Enabled {
			continue
		}
		if !state.policy.InsideWindow(now, a.loc) {
			continue
		}
		seen[server] = true

		a.notifyLimit(ctx, sess, server, state)
		a.notifyWindow(ctx, sess, server, state, now)
	}
}

// notifyLimit warns about the daily usage limit. Unlimited days are skipped.
func (a *Agent) notifyLimit(ctx context.Context, sess session.Session, server string, state *userState) {
	limit := state.policy.DailyLimitSeconds
	if limit <= 0 {
		return
	}
	remaining := limit - a.estimated(state)
	if remaining < 0 {
		remaining = 0
	}
	if minutes, ok := a.dueNotice(notify.KindLimit, server, limit, remaining, limitNoticeMinutes); ok {
		a.sendNotice(ctx, sess, server, notify.Notice{Kind: notify.KindLimit, Minutes: minutes})
	}
}

// notifyWindow warns about the end of the current permitted window.
func (a *Agent) notifyWindow(ctx context.Context, sess session.Session, server string, state *userState, now time.Time) {
	remaining, ok := windowRemaining(now, state.policy, a.loc)
	if !ok {
		return
	}
	seconds := int64(remaining / time.Second)
	if minutes, ok := a.dueNotice(notify.KindWindow, server, 0, seconds, windowNoticeMinutes); ok {
		a.sendNotice(ctx, sess, server, notify.Notice{Kind: notify.KindWindow, Minutes: minutes})
	}
}

// dueNotice returns the most urgent unsent threshold that has been reached and
// marks all less urgent thresholds as sent. This avoids replaying 15 and 5
// minutes when the Enforcer starts with only 3 minutes left.
func (a *Agent) dueNotice(kind notify.Kind, user string, limit, remaining int64, thresholds []int) (int, bool) {
	for _, threshold := range thresholds {
		if a.notified[noticeKey(user, kind, limit, threshold)] {
			continue
		}
		if remaining > int64(threshold)*60 {
			continue
		}
		for _, lessUrgent := range thresholds {
			if lessUrgent >= threshold {
				a.notified[noticeKey(user, kind, limit, lessUrgent)] = true
			}
		}
		return threshold, true
	}
	return 0, false
}

func noticeKey(user string, kind notify.Kind, limit int64, threshold int) string {
	return fmt.Sprintf("%s|%s|%d|%d", user, kind, limit, threshold)
}

func (a *Agent) sendNotice(ctx context.Context, sess session.Session, user string, notice notify.Notice) {
	if err := a.notifier.Notify(ctx, sess, notice); err != nil {
		log.Printf("WARN: notify user=%q session=%s: %v", user, sess.ID, err)
		return
	}
	log.Printf("NOTIFY: user=%q session=%s kind=%s minutes=%d", user, sess.ID, notice.Kind, notice.Minutes)
}

// windowRemaining returns the time until the current window ends.
func windowRemaining(now time.Time, p *timetower.Policy, loc *time.Location) (time.Duration, bool) {
	if p == nil || !p.Enabled {
		return 0, false
	}
	local := now.In(loc)
	day := weekdayDays[int(local.Weekday())]
	clock := policy.Clock(local.Hour()*60 + local.Minute())
	for _, window := range p.Windows {
		if window.Day != day || clock < window.Start || clock >= window.End {
			continue
		}
		end := time.Date(local.Year(), local.Month(), local.Day(),
			int(window.End)/60, int(window.End)%60, 0, 0, loc)
		remaining := end.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return remaining, true
	}
	return 0, false
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
		if sess.ID == "" || !a.isManaged(sess.User) {
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

func (a *Agent) hasActiveSession(sessions []session.Session, server string) bool {
	for _, sess := range sessions {
		if a.sessionMatchesServer(sess.User, server) && session.IsCountedSession(sess) && sess.Active && !sess.Locked {
			return true
		}
	}
	return false
}

func (a *Agent) sessionsForServer(sessions []session.Session, server string) []session.Session {
	var out []session.Session
	for _, sess := range sessions {
		if a.sessionMatchesServer(sess.User, server) {
			out = append(out, sess)
		}
	}
	return out
}

func (a *Agent) sessionMatchesServer(local, server string) bool {
	for _, user := range a.users {
		if user.Server == server && session.SameUser(user.Local, local) {
			return true
		}
	}
	return false
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
