package timetower

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"timelord/internal/policy"
)

const maxStatusBytes = 8 << 20

const (
	defaultHistoryDays = 7
	maxHistoryDays     = 400
)

const (
	adminUsername    = "admin"
	enforcerUsername = "enforcer"

	scheduleRealm = `Basic realm="Timetower"`
	adminRealm    = `Basic realm="Timetower Admin"`
	enforcerRealm = `Basic realm="Timetower Enforcer"`
)

type principalKind int

const (
	principalAdmin principalKind = iota
	principalEnforcer
)

type principal struct {
	username string
	kind     principalKind
}

func (p principal) isAdmin() bool {
	return p.kind == principalAdmin
}

func (p principal) isEnforcer() bool {
	return p.kind == principalEnforcer
}

// Server serves the Timetower REST API.
type Server struct {
	cfg              *Config
	store            *Store
	enforcerPassword string
	adminPassword    string
	now              func() time.Time
	mux              *http.ServeMux

	mu       sync.Mutex
	lastSeen map[string]time.Time
	online   map[string]bool
}

// NewServer returns a Timetower API server. enforcerPassword is the Basic auth
// password for the Enforcer username; adminPassword is the Basic auth password
// for the admin username.
func NewServer(cfg *Config, store *Store, enforcerPassword, adminPassword string) *Server {
	s := &Server{
		cfg:              cfg,
		store:            store,
		enforcerPassword: enforcerPassword,
		adminPassword:    adminPassword,
		now:              time.Now,
		lastSeen:         make(map[string]time.Time),
		online:           make(map[string]bool),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/schedule/", s.handleSchedule)
	mux.HandleFunc("/api/v1/status/", s.handleStatus)
	mux.HandleFunc("/api/v1/override/", s.handleOverride)
	mux.HandleFunc("/api/v1/enforcers", s.handleEnforcers)
	mux.HandleFunc("/api/v1/users", s.handleUsers)
	mux.HandleFunc("/api/v1/history/", s.handleHistory)
	s.mux = mux
	return s
}

// ServeHTTP implements http.Handler. Every client uses HTTP Basic auth; the
// username determines whether it is an admin or an Enforcer. The schedule
// endpoint accepts both roles.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/v1/schedule/"):
		p, ok := s.authenticate(r)
		if !ok {
			s.unauthorized(w, scheduleRealm)
			return
		}
		if !p.isAdmin() && !p.isEnforcer() {
			s.forbidden(w)
			return
		}
	case strings.HasPrefix(r.URL.Path, "/api/v1/status/"):
		p, ok := s.authenticate(r)
		if !ok {
			s.unauthorized(w, enforcerRealm)
			return
		}
		if !p.isEnforcer() {
			s.forbidden(w)
			return
		}
	case strings.HasPrefix(r.URL.Path, "/api/v1/override/"),
		strings.HasPrefix(r.URL.Path, "/api/v1/history/"),
		r.URL.Path == "/api/v1/enforcers", r.URL.Path == "/api/v1/users":
		p, ok := s.authenticate(r)
		if !ok {
			s.unauthorized(w, adminRealm)
			return
		}
		if !p.isAdmin() {
			s.forbidden(w)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) unauthorized(w http.ResponseWriter, challenge string) {
	w.Header().Set("WWW-Authenticate", challenge)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func (s *Server) forbidden(w http.ResponseWriter) {
	http.Error(w, "forbidden", http.StatusForbidden)
}

// AdminOnly wraps next with the admin Basic auth check. It is used for the
// static admin UI, while the API handler performs its own per-endpoint checks.
func (s *Server) AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Authorization")
		p, ok := s.authenticate(r)
		if !ok {
			s.unauthorized(w, adminRealm)
			return
		}
		if !p.isAdmin() {
			s.forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate resolves HTTP Basic credentials to a principal. The username is
// the role selector; unknown usernames and wrong passwords are both rejected.
func (s *Server) authenticate(r *http.Request) (principal, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return principal{}, false
	}

	var expected string
	var kind principalKind
	switch username {
	case adminUsername:
		expected, kind = s.adminPassword, principalAdmin
	case enforcerUsername:
		expected, kind = s.enforcerPassword, principalEnforcer
	default:
		return principal{}, false
	}
	if expected == "" {
		return principal{}, false
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(expected)) != 1 {
		return principal{}, false
	}
	return principal{username: username, kind: kind}, true
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userName, ok := userFromPath(r, "/api/v1/schedule/")
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	user, ok := s.cfg.UserByName(userName)
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	now := s.now().UTC()
	loc := s.cfg.Location()
	day := now.In(loc).Format(dateLayout)
	policy := s.policyForDay(user, policyDay(now, loc), day)
	used := s.store.AggregateUsage(user.Name, day)
	response := Schedule{
		SchemaVersion:         SchemaVersion,
		ServerTime:            now,
		Timezone:              s.cfg.Timezone,
		StatusIntervalSeconds: s.cfg.StatusIntervalSeconds,
		OfflineGraceSeconds:   s.cfg.OfflineGraceSeconds,
		User:                  user.Name,
		Policy:                policy,
		Usage: UsageState{
			Day:              day,
			UsedSeconds:      used,
			RemainingSeconds: remainingSeconds(policy, used),
		},
	}

	writeRevision(w, s.revision(day))
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleOverride(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userName, ok := userFromPath(r, "/api/v1/override/")
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	user, ok := s.cfg.UserByName(userName)
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if user.Policy == nil {
		http.Error(w, "user has no policy", http.StatusUnprocessableEntity)
		return
	}

	var request OverrideRequest
	if err := decodeJSONStrict(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validateOverride(&request); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	now := s.now().UTC()
	loc := s.cfg.Location()
	day := now.In(loc).Format(dateLayout)
	weekday := policyDay(now, loc)
	configured, hasWindow := user.Policy.DayWindow(weekday)
	window := configured
	if !hasWindow {
		window = policy.Window{Day: weekday, Start: 0, End: policy.MinutesPerDay - 1}
	}

	next, _ := s.store.Override(user.Name, day)
	if request.Start != nil {
		if hasWindow && *request.Start == configured.Start {
			next.Start = nil
		} else {
			next.Start = request.Start
		}
	}
	if request.End != nil {
		if hasWindow && *request.End == configured.End {
			next.End = nil
		} else {
			next.End = request.End
		}
	}
	if request.LimitMinutes != nil {
		if *request.LimitMinutes == defaultLimitMinutes(user, weekday) {
			next.LimitMinutes = nil
		} else {
			next.LimitMinutes = request.LimitMinutes
		}
	}
	if !hasWindow && (request.Start != nil || request.End != nil || next.Start != nil || next.End != nil) {
		if next.Start == nil {
			start := window.Start
			next.Start = &start
		}
		if next.End == nil {
			end := window.End
			next.End = &end
		}
	}

	if next.Start != nil || next.End != nil {
		effective := window
		if next.Start != nil {
			effective.Start = *next.Start
		}
		if next.End != nil {
			effective.End = *next.End
		}
		if effective.Start >= effective.End {
			http.Error(w, "start must be earlier than end", http.StatusUnprocessableEntity)
			return
		}
	}

	if next.empty() {
		s.store.ClearOverride(user.Name, day)
	} else {
		s.store.SetOverride(user.Name, day, next)
	}
	if err := s.store.Save(); err != nil {
		log.Printf("WARN: save state: %v", err)
	}

	policy := s.policyForDay(user, weekday, day)
	var limit int64
	if policy != nil {
		limit = policy.DailyLimitSeconds / 60
	}
	writeRevision(w, s.revision(day))
	writeJSON(w, http.StatusOK, Override{
		User:         user.Name,
		Day:          day,
		Windows:      dayWindows(policy, weekday),
		LimitMinutes: limit,
	})
}

func defaultLimitMinutes(user *User, day policy.Day) int64 {
	p := user.Policy.ForDay(day)
	if p == nil {
		return 0
	}
	return p.DailyLimitSeconds / 60
}

func validateOverride(request *OverrideRequest) error {
	if request.Start == nil && request.End == nil && request.LimitMinutes == nil {
		return fmt.Errorf("at least one of start, end, or limit_minutes is required")
	}
	if request.Start != nil && !request.Start.Valid() {
		return fmt.Errorf("start is out of range")
	}
	if request.End != nil && !request.End.Valid() {
		return fmt.Errorf("end is out of range")
	}
	if request.LimitMinutes != nil {
		if *request.LimitMinutes < 0 {
			return fmt.Errorf("limit_minutes must not be negative")
		}
		if *request.LimitMinutes > maxLimitMinutes {
			return fmt.Errorf("limit_minutes is too large")
		}
	}
	return nil
}

func dayWindows(p *Policy, day policy.Day) []policy.Window {
	if p == nil {
		return []policy.Window{}
	}
	windows := make([]policy.Window, 0, len(p.Windows))
	for _, window := range p.Windows {
		if window.Day == day {
			windows = append(windows, window)
		}
	}
	return windows
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userName, ok := userFromPath(r, "/api/v1/status/")
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	user, ok := s.cfg.UserByName(userName)
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	var request StatusRequest
	if err := decodeJSONLenient(r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := validateStatusRequest(&request); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	now := s.now().UTC()
	loc := s.cfg.Location()
	serverDay := now.In(loc).Format(dateLayout)
	usageDay := request.ObservedAt.In(loc).Format(dateLayout)
	if usageDay > serverDay {
		http.Error(w, "observed_at is later than server day "+serverDay, http.StatusConflict)
		return
	}
	s.store.RecordUsage(request.Hostname, user.Name, usageDay, request.UsageSeconds)

	s.markSeen(request.Hostname, now)
	s.store.SetStatus(request.Hostname, user.Name, StatusRecord{
		Hostname:   request.Hostname,
		User:       user.Name,
		ReceivedAt: now,
		Request:    request,
	})
	if err := s.store.Save(); err != nil {
		log.Printf("WARN: save state: %v", err)
	}

	response := s.statusResponse(user, request.Hostname, now, serverDay)
	writeRevision(w, s.revision(serverDay))
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleEnforcers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.enforcerList(s.now().UTC()))
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	users := make([]UserInfo, 0, len(s.cfg.Users))
	for i := range s.cfg.Users {
		user := &s.cfg.Users[i]
		users = append(users, UserInfo{User: user.Name, DisplayName: user.DisplayName})
	}
	writeJSON(w, http.StatusOK, UserList{Users: users})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userName, ok := userFromPath(r, "/api/v1/history/")
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	user, ok := s.cfg.UserByName(userName)
	if !ok {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	days, err := parseHistoryDays(r.URL.Query().Get("days"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := s.now().UTC()
	loc := s.cfg.Location()
	end, err := time.ParseInLocation(dateLayout, now.In(loc).Format(dateLayout), loc)
	if err != nil {
		http.Error(w, "invalid server day", http.StatusInternalServerError)
		return
	}
	start := end.AddDate(0, 0, -(days - 1))
	totals := s.store.AggregateUsageRange(user.Name, start.Format(dateLayout), end.Format(dateLayout))

	history := make([]UsageDay, 0, days)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		key := day.Format(dateLayout)
		history = append(history, UsageDay{Day: key, UsedSeconds: totals[key]})
	}
	writeJSON(w, http.StatusOK, UsageHistory{User: user.Name, History: history})
}

func parseHistoryDays(raw string) (int, error) {
	if raw == "" {
		return defaultHistoryDays, nil
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 1 || days > maxHistoryDays {
		return 0, fmt.Errorf("days must be an integer between 1 and %d", maxHistoryDays)
	}
	return days, nil
}

// Monitor logs Enforcer connection and heartbeat-loss transitions.
func (s *Server) Monitor(ctx context.Context) {
	interval := time.Duration(s.cfg.StatusIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.checkTimeouts(now.UTC())
		}
	}
}

func (s *Server) markSeen(hostname string, now time.Time) {
	s.mu.Lock()
	wasOnline := s.online[hostname]
	s.online[hostname] = true
	s.lastSeen[hostname] = now
	s.mu.Unlock()

	if !wasOnline {
		log.Printf("INFO: enforcer %q connected", hostname)
	}
}

func (s *Server) checkTimeouts(now time.Time) {
	timeout := 3 * time.Duration(s.cfg.StatusIntervalSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}

	type lostEnforcer struct {
		hostname string
		last     time.Time
	}
	var lost []lostEnforcer

	s.mu.Lock()
	for hostname, last := range s.lastSeen {
		if s.online[hostname] && now.Sub(last) > timeout {
			s.online[hostname] = false
			lost = append(lost, lostEnforcer{hostname: hostname, last: last})
		}
	}
	s.mu.Unlock()

	for _, enforcer := range lost {
		log.Printf("WARN: enforcer %q heartbeat lost (last seen %s)", enforcer.hostname, enforcer.last.UTC().Format(time.RFC3339))
	}
}

func (s *Server) presence(hostname string) (bool, *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	online := s.online[hostname]
	last, ok := s.lastSeen[hostname]
	if !ok {
		return online, nil
	}
	last = last.UTC()
	return online, &last
}

func (s *Server) enforcerList(now time.Time) EnforcerList {
	loc := s.cfg.Location()
	day := now.In(loc).Format(dateLayout)
	today := policyDay(now, loc)
	response := EnforcerList{
		ServerTime: now,
		ServerDay:  day,
		Enforcers:  make([]EnforcerStatus, 0),
	}

	records := s.store.StatusRecords()
	byHostname := make(map[string][]StatusRecord)
	lastKnown := make(map[string]time.Time)
	for _, record := range records {
		byHostname[record.Hostname] = append(byHostname[record.Hostname], record)
		if record.ReceivedAt.After(lastKnown[record.Hostname]) {
			lastKnown[record.Hostname] = record.ReceivedAt
		}
	}

	hostnames := make([]string, 0, len(byHostname))
	for hostname := range byHostname {
		hostnames = append(hostnames, hostname)
	}
	sort.Strings(hostnames)

	for _, hostname := range hostnames {
		online, lastSeen := s.presence(hostname)
		if lastSeen == nil {
			last := lastKnown[hostname].UTC()
			lastSeen = &last
		}
		status := EnforcerStatus{
			Hostname: hostname,
			Online:   online,
			LastSeen: lastSeen,
			Users:    make([]UserStatus, 0, len(byHostname[hostname])),
		}
		for _, record := range byHostname[hostname] {
			user, ok := s.cfg.UserByName(record.User)
			if !ok {
				continue
			}
			policy := s.policyForDay(user, today, day)
			used := s.store.AggregateUsage(user.Name, day)
			inside := policy.InsideWindow(now, loc)
			allowed := policy.Allowed(now, used, loc)
			var limit int64
			if policy != nil {
				limit = policy.DailyLimitSeconds
			}
			status.Users = append(status.Users, UserStatus{
				User:              user.Name,
				UsedSeconds:       used,
				DailyLimitSeconds: limit,
				RemainingSeconds:  remainingSeconds(policy, used),
				InsideWindow:      inside,
				Allowed:           allowed,
				DenyReason:        denyReason(policy, inside, limit, used),
			})
		}
		response.Enforcers = append(response.Enforcers, status)
	}
	return response
}

func (s *Server) statusResponse(user *User, hostname string, now time.Time, serverDay string) StatusResponse {
	loc := s.cfg.Location()
	policy := s.policyForDay(user, policyDay(now, loc), serverDay)
	reported := s.store.Usage(hostname, user.Name, serverDay)
	aggregate := s.store.AggregateUsage(user.Name, serverDay)
	inside := policy.InsideWindow(now, loc)
	allowed := policy.Allowed(now, aggregate, loc)
	var limit int64
	if policy != nil {
		limit = policy.DailyLimitSeconds
	}
	return StatusResponse{
		ServerTime:           now,
		ServerDay:            serverDay,
		User:                 user.Name,
		CurrentDay:           serverDay,
		ReportedUsedSeconds:  reported,
		AggregateUsedSeconds: aggregate,
		DailyLimitSeconds:    limit,
		RemainingSeconds:     remainingSeconds(policy, aggregate),
		InsideWindow:         inside,
		Allowed:              allowed,
		DenyReason:           denyReason(policy, inside, limit, aggregate),
	}
}

// policyForDay returns a user's policy for one weekday with the current-day
// override applied, if one is set for date.
func (s *Server) policyForDay(user *User, day policy.Day, date string) *Policy {
	p := user.Policy.ForDay(day)
	if p == nil {
		return nil
	}
	override, ok := s.store.Override(user.Name, date)
	if !ok {
		return p
	}
	if override.LimitMinutes != nil {
		p.DailyLimitSeconds = *override.LimitMinutes * 60
	}
	if override.Start != nil || override.End != nil {
		window := user.Policy.DayWindowOrDefault(day)
		if override.Start != nil {
			window.Start = *override.Start
		}
		if override.End != nil {
			window.End = *override.End
		}
		p.Windows = []policy.Window{window}
	}
	return p
}

// revision combines the static schedule revision, the day, and the day's
// override state. Enforcers refetch schedules whenever it changes.
func (s *Server) revision(day string) int64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d:%d", s.cfg.RevisionForDay(day), s.store.OverridesRevision(day))
	return int64(h.Sum64() >> 1)
}

func userFromPath(r *http.Request, prefix string) (string, bool) {
	raw := strings.TrimPrefix(r.URL.Path, prefix)
	if raw == "" || strings.Contains(raw, "/") {
		return "", false
	}
	user, err := url.PathUnescape(raw)
	if err != nil || user == "" || strings.Contains(user, "/") {
		return "", false
	}
	return user, true
}

func validateStatusRequest(request *StatusRequest) error {
	if request.ObservedAt.IsZero() {
		return fmt.Errorf("observed_at is required")
	}
	if request.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if len(request.Hostname) > 256 {
		return fmt.Errorf("hostname is longer than 256 bytes")
	}
	if request.SessionCount < 0 {
		return fmt.Errorf("session_count must not be negative")
	}
	if request.UsageSeconds < 0 {
		return fmt.Errorf("usage_seconds must not be negative")
	}
	if request.CollectedInMS < 0 {
		return fmt.Errorf("collected_in_ms must not be negative")
	}
	for i, process := range request.Processes {
		switch process.Scope {
		case "app", "system":
		default:
			return fmt.Errorf("processes[%d]: scope %q is invalid", i, process.Scope)
		}
		if process.Instances < 0 {
			return fmt.Errorf("processes[%d]: instances must not be negative", i)
		}
		if process.MemoryRSSBytes < 0 {
			return fmt.Errorf("processes[%d]: memory_rss_bytes must not be negative", i)
		}
		if process.CPUSecondsTotal < 0 {
			return fmt.Errorf("processes[%d]: cpu_seconds_total must not be negative", i)
		}
	}
	return nil
}

func remainingSeconds(policy *Policy, used int64) *int64 {
	if policy == nil || policy.DailyLimitSeconds == 0 {
		return nil
	}
	remaining := policy.DailyLimitSeconds - used
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}

func denyReason(policy *Policy, inside bool, limit, used int64) *string {
	var reason string
	switch {
	case policy == nil:
		reason = "no_policy"
	case !policy.Enabled:
		reason = "disabled"
	case !inside:
		reason = "outside_window"
	case limit > 0 && used >= limit:
		reason = "daily_limit_reached"
	default:
		return nil
	}
	return &reason
}

func writeRevision(w http.ResponseWriter, revision int64) {
	w.Header().Set("X-Schedule-Revision", strconv.FormatInt(revision, 10))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("WARN: write response: %v", err)
	}
}

// decodeJSONStrict rejects unknown fields. Use it for operator-controlled
// input, where a misspelled field would otherwise be silently ignored.
func decodeJSONStrict(r *http.Request, value any) error {
	return decodeJSON(r, value, true)
}

// decodeJSONLenient ignores unknown fields. Use it for machine-to-machine
// input that must survive rolling client/server upgrades.
func decodeJSONLenient(r *http.Request, value any) error {
	return decodeJSON(r, value, false)
}

func decodeJSON(r *http.Request, value any, strict bool) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxStatusBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxStatusBytes {
		return errPayloadTooLarge
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("unexpected data after JSON object")
	}
	return nil
}

var errPayloadTooLarge = errors.New("request body too large")

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errPayloadTooLarge) {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}
