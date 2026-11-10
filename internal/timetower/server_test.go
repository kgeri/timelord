package timetower

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestScheduleHandler(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Schedule-Revision"); got == "" {
		t.Fatal("X-Schedule-Revision is empty")
	}

	var response Schedule
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode schedule: %v", err)
	}
	if response.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", response.SchemaVersion, SchemaVersion)
	}
	if response.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", response.Timezone)
	}
	if len(response.Subjects) != 1 {
		t.Fatalf("len(Subjects) = %d, want 1", len(response.Subjects))
	}
	subject := response.Subjects[0]
	if subject.Subject != "dani" || subject.Account != "dani" {
		t.Errorf("subject = %+v, want dani/dani", subject)
	}
	if subject.Usage.Day != "2024-06-03" {
		t.Errorf("Usage.Day = %q, want 2024-06-03", subject.Usage.Day)
	}
	if subject.Usage.UsedSeconds != 0 {
		t.Errorf("Usage.UsedSeconds = %d, want 0", subject.Usage.UsedSeconds)
	}
	if subject.Usage.RemainingSeconds == nil || *subject.Usage.RemainingSeconds != 3600 {
		t.Errorf("Usage.RemainingSeconds = %v, want 3600", subject.Usage.RemainingSeconds)
	}
}

func TestScheduleUsesForwardedClientIP(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 192.0.2.10")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleUnknownEnforcer(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule", nil)
	req.RemoteAddr = "192.0.2.99:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestScheduleMethodNotAllowed(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedule", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow = %q, want GET", got)
	}
}

func TestStatusHandlerRecordsMonotonicUsage(t *testing.T) {
	server, store := newTestServer(t, testNow())

	rec := putStatus(t, server, statusBody(1200))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Schedule-Revision"); got == "" {
		t.Fatal("X-Schedule-Revision is empty")
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(response.Subjects) != 1 {
		t.Fatalf("len(Subjects) = %d, want 1", len(response.Subjects))
	}
	subject := response.Subjects[0]
	if subject.ReportedUsedSeconds != 1200 || subject.AggregateUsedSeconds != 1200 {
		t.Errorf("usage = reported %d aggregate %d, want 1200/1200", subject.ReportedUsedSeconds, subject.AggregateUsedSeconds)
	}
	if subject.RemainingSeconds == nil || *subject.RemainingSeconds != 2400 {
		t.Errorf("RemainingSeconds = %v, want 2400", subject.RemainingSeconds)
	}
	if !subject.Allowed {
		t.Error("Allowed = false, want true")
	}
	if subject.DenyReason != nil {
		t.Errorf("DenyReason = %v, want nil", *subject.DenyReason)
	}

	// A lower counter must not replace the stored value.
	rec = putStatus(t, server, statusBody(1000))
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	response = StatusResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode second status: %v", err)
	}
	if got := response.Subjects[0].ReportedUsedSeconds; got != 1200 {
		t.Errorf("ReportedUsedSeconds after lower report = %d, want 1200", got)
	}
	if got := store.Usage("test-pc", "dani", "2024-06-03"); got != 1200 {
		t.Errorf("stored Usage = %d, want 1200", got)
	}
}

func TestStatusHandlerDailyLimitReached(t *testing.T) {
	now := testNow()
	server, _ := newTestServer(t, now)

	rec := putStatus(t, server, statusBody(3600))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	subject := response.Subjects[0]
	if subject.Allowed {
		t.Error("Allowed = true, want false")
	}
	if subject.DenyReason == nil || *subject.DenyReason != "daily_limit_reached" {
		t.Errorf("DenyReason = %v, want daily_limit_reached", subject.DenyReason)
	}
	if subject.RemainingSeconds == nil || *subject.RemainingSeconds != 0 {
		t.Errorf("RemainingSeconds = %v, want 0", subject.RemainingSeconds)
	}
}

func TestStatusHandlerDayMismatch(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	rec := putStatus(t, server, statusBodyWithDay(100, "2024-06-04"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatusHandlerRejectsInvalidJSON(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status", strings.NewReader("{"))
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestStatusHandlerRejectsUnknownField(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	body := strings.Replace(statusBody(100), `"processes"`, `"unexpected":true,"processes"`, 1)
	rec := putStatus(t, server, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatusHandlerIgnoresUnassignedSubject(t *testing.T) {
	server, store := newTestServer(t, testNow())

	body := `{
		"observed_at": "2024-06-03T10:00:00Z",
		"enforcer_version": "0.2.0",
		"health": {"status": "ok", "last_error": null},
		"subjects": [
			{"subject": "other", "account": "other", "account_present": true,
			 "session_count": 1, "active": true, "locked": false,
			 "usage": [{"day": "2024-06-03", "used_seconds": 500}]}
		],
		"processes": []
	}`
	rec := putStatus(t, server, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if len(response.Subjects) != 1 || response.Subjects[0].Subject != "dani" {
		t.Fatalf("Subjects = %+v, want only dani", response.Subjects)
	}
	if got := store.AggregateUsage("other", "2024-06-03"); got != 0 {
		t.Errorf("unassigned usage = %d, want 0", got)
	}
}

func TestStatusHandlerOutsideWindow(t *testing.T) {
	now := time.Date(2024, time.June, 3, 20, 0, 0, 0, time.UTC)
	configJSON := strings.Replace(testConfigJSON,
		`"00:00", "end": "23:59"`, `"08:00", "end": "12:00"`, 1)
	server, _ := newTestServerWithConfig(t, configJSON, now)

	rec := putStatus(t, server, statusBody(100))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	subject := response.Subjects[0]
	if subject.Allowed {
		t.Error("Allowed = true, want false")
	}
	if subject.DenyReason == nil || *subject.DenyReason != "outside_window" {
		t.Errorf("DenyReason = %v, want outside_window", subject.DenyReason)
	}
}

func TestAggregateUsageAcrossEnforcers(t *testing.T) {
	configJSON := `{
		"timezone": "UTC",
		"status_interval_seconds": 60,
		"offline_grace_seconds": 3600,
		"subjects": [
			{"id": "dani", "policy": {"enabled": true, "daily_limit_seconds": 3600,
			 "windows": [{"day": "mon", "start": "00:00", "end": "23:59"}]}}
		],
		"enforcers": [
			{"ip": "192.0.2.10", "name": "pc-one", "accounts": [{"subject": "dani", "account": "dani"}]},
			{"ip": "192.0.2.11", "name": "pc-two", "accounts": [{"subject": "dani", "account": "dani"}]}
		]
	}`
	server, _ := newTestServerWithConfig(t, configJSON, testNow())

	rec := putStatus(t, server, statusBody(1000))
	if rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec.Code)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status", strings.NewReader(statusBody(500)))
	req.RemoteAddr = "192.0.2.11:1234"
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode second status: %v", err)
	}
	subject := response.Subjects[0]
	if subject.ReportedUsedSeconds != 500 {
		t.Errorf("ReportedUsedSeconds = %d, want 500", subject.ReportedUsedSeconds)
	}
	if subject.AggregateUsedSeconds != 1500 {
		t.Errorf("AggregateUsedSeconds = %d, want 1500", subject.AggregateUsedSeconds)
	}
}

func newTestServer(t *testing.T, now time.Time) (*Server, *Store) {
	t.Helper()
	return newTestServerWithConfig(t, testConfigJSON, now)
}

func newTestServerWithConfig(t *testing.T, configJSON string, now time.Time) (*Server, *Store) {
	t.Helper()
	config, err := LoadConfig(writeConfig(t, configJSON))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	server := NewServer(config, store)
	server.now = func() time.Time { return now }
	return server, store
}

func putStatus(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status", strings.NewReader(body))
	req.RemoteAddr = "192.0.2.10:1234"
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func statusBody(used int64) string {
	return statusBodyWithDay(used, "2024-06-03")
}

func statusBodyWithDay(used int64, day string) string {
	return `{
		"observed_at": "2024-06-03T10:00:00Z",
		"enforcer_version": "0.2.0",
		"health": {"status": "ok", "last_error": null},
		"subjects": [
			{"subject": "dani", "account": "dani", "account_present": true,
			 "session_count": 1, "active": true, "locked": false,
			 "usage": [{"day": "` + day + `", "used_seconds": ` + itoa(used) + `}]}
		],
		"processes": [
			{"user": "dani", "name": "firefox", "scope": "app",
			 "instances": 1, "memory_rss_bytes": 100, "cpu_seconds_total": 2.5}
		]
	}`
}

func itoa(value int64) string {
	return strconv.FormatInt(value, 10)
}

func testNow() time.Time {
	return time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
}
