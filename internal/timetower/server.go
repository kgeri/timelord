package timetower

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
	now   func() time.Time
	mux   *http.ServeMux

	mu       sync.Mutex
	lastSeen map[string]time.Time
	online   map[string]bool
}

// NewServer returns a Timetower API server.
func NewServer(cfg *Config, store *Store) *Server {
	s := &Server{
		cfg:      cfg,
		store:    store,
		now:      time.Now,
		lastSeen: make(map[string]time.Time),
		online:   make(map[string]bool),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/schedule", s.handleSchedule)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/enforcers", s.handleEnforcers)
	s.mux = mux
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	enforcer, ok := s.enforcerForRequest(r)
	if !ok {
		s.rejectUnknownEnforcer(w, r)
		return
	}

	now := s.now().UTC()
	s.markSeen(enforcer, now)
	day := now.In(s.cfg.Location()).Format(dateLayout)
	response := Schedule{
		SchemaVersion:         SchemaVersion,
		ServerTime:            now,
		Timezone:              s.cfg.Timezone,
		StatusIntervalSeconds: s.cfg.StatusIntervalSeconds,
		OfflineGraceSeconds:   s.cfg.OfflineGraceSeconds,
		Subjects:              make([]ScheduleSubject, 0, len(enforcer.Accounts)),
	}
	for _, account := range enforcer.Accounts {
		subject, ok := s.cfg.SubjectByID(account.Subject)
		if !ok {
			continue
		}
		used := s.store.AggregateUsage(subject.ID, day)
		response.Subjects = append(response.Subjects, ScheduleSubject{
			Subject: subject.ID,
			Account: account.Account,
			Policy:  subject.Policy,
			Usage: UsageState{
				Day:              day,
				UsedSeconds:      used,
				RemainingSeconds: remainingSeconds(subject.Policy, used),
			},
		})
	}

	writeRevision(w, s.cfg.Revision())
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	enforcer, ok := s.enforcerForRequest(r)
	if !ok {
		s.rejectUnknownEnforcer(w, r)
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
	serverDay := now.In(s.cfg.Location()).Format(dateLayout)
	acknowledged, err := s.recordUsage(enforcer, request, serverDay)
	if err != nil {
		var mismatch *dayMismatchError
		if errors.As(err, &mismatch) {
			http.Error(w, mismatch.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	s.markSeen(enforcer, now)
	s.store.SetStatus(enforcer.Name, StatusRecord{
		Enforcer:   enforcer.Name,
		ReceivedAt: now,
		Request:    request,
	})
	if err := s.store.Save(); err != nil {
		log.Printf("WARN: save state: %v", err)
	}

	response := s.statusResponse(enforcer, acknowledged, now, serverDay)
	writeRevision(w, s.cfg.Revision())
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

func (s *Server) markSeen(enforcer *Enforcer, now time.Time) {
	s.mu.Lock()
	wasOnline := s.online[enforcer.Name]
	s.online[enforcer.Name] = true
	s.lastSeen[enforcer.Name] = now
	s.mu.Unlock()

	if !wasOnline {
		log.Printf("INFO: enforcer %q connected from %s", enforcer.Name, enforcer.IP)
	}
}

func (s *Server) checkTimeouts(now time.Time) {
	timeout := 3 * time.Duration(s.cfg.StatusIntervalSeconds) * time.Second
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}

	type lostEnforcer struct {
		name string
		last time.Time
	}
	var lost []lostEnforcer

	s.mu.Lock()
	for name, last := range s.lastSeen {
		if s.online[name] && now.Sub(last) > timeout {
			s.online[name] = false
			lost = append(lost, lostEnforcer{name: name, last: last})
		}
	}
	s.mu.Unlock()

	for _, enforcer := range lost {
		log.Printf("WARN: enforcer %q heartbeat lost (last seen %s)", enforcer.name, enforcer.last.UTC().Format(time.RFC3339))
	}
}

func (s *Server) presence(name string) (bool, *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	online := s.online[name]
	last, ok := s.lastSeen[name]
	if !ok {
		return online, nil
	}
	last = last.UTC()
	return online, &last
}

func (s *Server) enforcerList(now time.Time) EnforcerList {
	loc := s.cfg.Location()
	day := now.In(loc).Format(dateLayout)
	response := EnforcerList{
		ServerTime: now,
		ServerDay:  day,
		Enforcers:  make([]EnforcerStatus, 0, len(s.cfg.Enforcers)),
	}

	for i := range s.cfg.Enforcers {
		enforcer := &s.cfg.Enforcers[i]
		online, lastSeen := s.presence(enforcer.Name)
		status := EnforcerStatus{
			IP:       enforcer.IP,
			Name:     enforcer.Name,
			Online:   online,
			LastSeen: lastSeen,
			Subjects: make([]EnforcerSubjectStatus, 0, len(enforcer.Accounts)),
		}
		for _, account := range enforcer.Accounts {
			subject, ok := s.cfg.SubjectByID(account.Subject)
			if !ok {
				continue
			}
			policy := subject.Policy
			used := s.store.AggregateUsage(subject.ID, day)
			inside := policy.InsideWindow(now, loc)
			allowed := policy.Allowed(now, used, loc)
			var limit int64
			if policy != nil {
				limit = policy.DailyLimitSeconds
			}
			status.Subjects = append(status.Subjects, EnforcerSubjectStatus{
				Subject:           subject.ID,
				Account:           account.Account,
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

func (s *Server) statusResponse(enforcer *Enforcer, acknowledged map[string][]string, now time.Time, serverDay string) StatusResponse {
	loc := s.cfg.Location()
	response := StatusResponse{
		ServerTime: now,
		ServerDay:  serverDay,
		Subjects:   make([]StatusResponseSubject, 0, len(enforcer.Accounts)),
	}
	for _, account := range enforcer.Accounts {
		subject, ok := s.cfg.SubjectByID(account.Subject)
		if !ok {
			continue
		}
		policy := subject.Policy
		reported := s.store.Usage(enforcer.Name, subject.ID, serverDay)
		aggregate := s.store.AggregateUsage(subject.ID, serverDay)
		inside := policy.InsideWindow(now, loc)
		allowed := policy.Allowed(now, aggregate, loc)
		var limit int64
		if policy != nil {
			limit = policy.DailyLimitSeconds
		}
		days := acknowledged[subject.ID]
		if days == nil {
			days = []string{}
		}
		response.Subjects = append(response.Subjects, StatusResponseSubject{
			Subject:              subject.ID,
			CurrentDay:           serverDay,
			ReportedUsedSeconds:  reported,
			AggregateUsedSeconds: aggregate,
			DailyLimitSeconds:    limit,
			RemainingSeconds:     remainingSeconds(policy, aggregate),
			InsideWindow:         inside,
			Allowed:              allowed,
			DenyReason:           denyReason(policy, inside, limit, aggregate),
			AcknowledgedDays:     days,
		})
	}
	return response
}

func (s *Server) recordUsage(enforcer *Enforcer, request StatusRequest, serverDay string) (map[string][]string, error) {
	assigned := make(map[string]bool, len(enforcer.Accounts))
	for _, account := range enforcer.Accounts {
		assigned[account.Subject] = true
	}

	acknowledged := make(map[string][]string)
	seenBySubject := make(map[string]map[string]bool)
	for _, subject := range request.Subjects {
		if !assigned[subject.Subject] {
			continue
		}
		seen := seenBySubject[subject.Subject]
		if seen == nil {
			seen = make(map[string]bool, len(subject.Usage))
			seenBySubject[subject.Subject] = seen
		}
		for _, usage := range subject.Usage {
			if usage.Day > serverDay {
				return nil, &dayMismatchError{serverDay: serverDay}
			}
			s.store.RecordUsage(enforcer.Name, subject.Subject, usage.Day, usage.UsedSeconds)
			if !seen[usage.Day] {
				acknowledged[subject.Subject] = append(acknowledged[subject.Subject], usage.Day)
				seen[usage.Day] = true
			}
		}
	}
	return acknowledged, nil
}

func (s *Server) rejectUnknownEnforcer(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	log.Printf("WARN: unknown enforcer ip=%q remote=%q x-forwarded-for=%q x-real-ip=%q forwarded=%q",
		ip, r.RemoteAddr, r.Header.Get("X-Forwarded-For"), r.Header.Get("X-Real-IP"), r.Header.Get("Forwarded"))
	http.Error(w, "unknown enforcer", http.StatusForbidden)
}

func (s *Server) enforcerForRequest(r *http.Request) (*Enforcer, bool) {
	// Try every address in the forwarded chain. Traefik may append its own
	// peer address after the real client, and a k3s service hop can add
	// another address. Only configured Enforcer addresses are accepted.
	for _, ip := range clientIPs(r) {
		if enforcer, ok := s.cfg.EnforcerByIP(ip.String()); ok {
			return enforcer, true
		}
	}
	return nil, false
}

// clientIPs returns the resolved client addresses in preference order. The
// X-Forwarded-For chain is walked from right to left, followed by X-Real-IP and
// the direct peer address.
func clientIPs(r *http.Request) []net.IP {
	var addresses []net.IP
	seen := make(map[string]bool)

	add := func(value string) {
		ip := parseIP(strings.TrimSpace(value))
		if ip == nil {
			return
		}
		key := ip.String()
		if seen[key] {
			return
		}
		seen[key] = true
		addresses = append(addresses, ip)
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			add(parts[i])
		}
	}
	add(r.Header.Get("X-Real-IP"))

	host := r.RemoteAddr
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	add(host)
	return addresses
}

func clientIP(r *http.Request) net.IP {
	addresses := clientIPs(r)
	if len(addresses) == 0 {
		return nil
	}
	return addresses[0]
}

func parseIP(value string) net.IP {
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = value[1 : len(value)-1]
	}
	if i := strings.LastIndex(value, "%"); i >= 0 {
		value = value[:i]
	}
	return net.ParseIP(value)
}

func validateStatusRequest(request *StatusRequest) error {
	if request.ObservedAt.IsZero() {
		return fmt.Errorf("observed_at is required")
	}
	switch request.Health.Status {
	case "ok", "degraded", "starting":
	default:
		return fmt.Errorf("health.status %q is invalid", request.Health.Status)
	}
	if request.Health.LastError != nil && len(*request.Health.LastError) > 1024 {
		return fmt.Errorf("health.last_error is longer than 1024 bytes")
	}
	if request.CollectedInMS < 0 {
		return fmt.Errorf("collected_in_ms must not be negative")
	}
	for i, subject := range request.Subjects {
		if subject.Subject == "" {
			return fmt.Errorf("subjects[%d]: subject is empty", i)
		}
		if subject.SessionCount < 0 {
			return fmt.Errorf("subjects[%d]: session_count must not be negative", i)
		}
		if len(subject.Account) > 256 {
			return fmt.Errorf("subjects[%d]: account is longer than 256 bytes", i)
		}
		if len(subject.Usage) > 7 {
			return fmt.Errorf("subjects[%d]: usage has more than 7 entries", i)
		}
		for j, usage := range subject.Usage {
			if _, err := time.Parse(dateLayout, usage.Day); err != nil {
				return fmt.Errorf("subjects[%d].usage[%d]: invalid day %q", i, j, usage.Day)
			}
			if usage.UsedSeconds < 0 {
				return fmt.Errorf("subjects[%d].usage[%d]: used_seconds must not be negative", i, j)
			}
		}
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

type dayMismatchError struct {
	serverDay string
}

func (e *dayMismatchError) Error() string {
	return "usage day is later than server day " + e.serverDay
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
