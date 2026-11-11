package timetower

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxStatusBytes = 8 << 20

// Server serves the Timetower REST API.
type Server struct {
	cfg   *Config
	store *Store
	key   string
	now   func() time.Time
	mux   *http.ServeMux

	mu       sync.Mutex
	lastSeen map[string]time.Time
	online   map[string]bool
}

// NewServer returns a Timetower API server. key is the shared bearer key.
func NewServer(cfg *Config, store *Store, key string) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		key:      key,
		now:      time.Now,
		lastSeen: make(map[string]time.Time),
		online:   make(map[string]bool),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/schedule/", s.handleSchedule)
	mux.HandleFunc("/api/v1/status/", s.handleStatus)
	mux.HandleFunc("/api/v1/enforcers", s.handleEnforcers)
	s.mux = mux
	return s
}

// ServeHTTP implements http.Handler. The schedule and status endpoints require
// the shared bearer key; the Enforcer listing is public for now.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/v1/schedule/") || strings.HasPrefix(r.URL.Path, "/api/v1/status/") {
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.key == "" {
		return false
	}
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(header, prefix)), []byte(s.key)) == 1
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
	policy := user.Policy.ForDay(policyDay(now, loc))
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

	writeRevision(w, s.cfg.RevisionForDay(day))
	writeJSON(w, http.StatusOK, response)
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
	if err := decodeJSON(r, &request); err != nil {
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
	writeRevision(w, s.cfg.RevisionForDay(serverDay))
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
			policy := user.Policy.ForDay(today)
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
	policy := user.Policy.ForDay(policyDay(now, loc))
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

func decodeJSON(r *http.Request, value any) error {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxStatusBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxStatusBytes {
		return errPayloadTooLarge
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
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
