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

const testKey = "test-key"

func TestScheduleHandler(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule/dani", nil)
	authorize(req)
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
	if response.User != "dani" {
		t.Errorf("User = %q, want dani", response.User)
	}
	if response.Usage.Day != "2024-06-03" {
		t.Errorf("Usage.Day = %q, want 2024-06-03", response.Usage.Day)
	}
	if response.Usage.UsedSeconds != 0 {
		t.Errorf("Usage.UsedSeconds = %d, want 0", response.Usage.UsedSeconds)
	}
	if response.Usage.RemainingSeconds == nil || *response.Usage.RemainingSeconds != 3600 {
		t.Errorf("Usage.RemainingSeconds = %v, want 3600", response.Usage.RemainingSeconds)
	}
}

func TestScheduleRequiresAuth(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule/dani", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/schedule/dani", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key status = %d, want 401", rec.Code)
	}
}

func TestScheduleUnknownUser(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/schedule/other", nil)
	authorize(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestScheduleMethodNotAllowed(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/schedule/dani", nil)
	authorize(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow = %q, want GET", got)
	}
}

func TestEnforcersListing(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	// The listing is public and empty before any status report.
	rec := getListing(server)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var listing EnforcerList
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	if len(listing.Enforcers) != 0 {
		t.Fatalf("len(Enforcers) = %d, want 0", len(listing.Enforcers))
	}

	if rec := putStatus(t, server, statusBody(1200)); rec.Code != http.StatusOK {
		t.Fatalf("status update = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	rec = getListing(server)
	listing = EnforcerList{}
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing after status: %v", err)
	}
	if len(listing.Enforcers) != 1 {
		t.Fatalf("len(Enforcers) = %d, want 1", len(listing.Enforcers))
	}
	enforcer := listing.Enforcers[0]
	if enforcer.Hostname != "test-host" || !enforcer.Online {
		t.Errorf("enforcer = %+v, want test-host online", enforcer)
	}
	if enforcer.LastSeen == nil {
		t.Error("LastSeen = nil, want timestamp")
	}
	if len(enforcer.Users) != 1 {
		t.Fatalf("len(Users) = %d, want 1", len(enforcer.Users))
	}
	user := enforcer.Users[0]
	if user.UsedSeconds != 1200 || user.RemainingSeconds == nil || *user.RemainingSeconds != 2400 {
		t.Errorf("user usage = %+v, want used 1200 remaining 2400", user)
	}
}

func TestHeartbeatTimeoutMarksOffline(t *testing.T) {
	server, _ := newTestServer(t, testNow())
	if rec := putStatus(t, server, statusBody(1200)); rec.Code != http.StatusOK {
		t.Fatalf("status update = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	server.checkTimeouts(testNow().Add(4 * time.Minute))

	rec := getListing(server)
	var listing EnforcerList
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	if listing.Enforcers[0].Online {
		t.Error("Online = true after heartbeat timeout, want false")
	}
	if listing.Enforcers[0].LastSeen == nil {
		t.Error("LastSeen = nil, want the previous heartbeat time")
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
	if response.User != "dani" {
		t.Errorf("User = %q, want dani", response.User)
	}
	if response.ReportedUsedSeconds != 1200 || response.AggregateUsedSeconds != 1200 {
		t.Errorf("usage = reported %d aggregate %d, want 1200/1200", response.ReportedUsedSeconds, response.AggregateUsedSeconds)
	}
	if response.RemainingSeconds == nil || *response.RemainingSeconds != 2400 {
		t.Errorf("RemainingSeconds = %v, want 2400", response.RemainingSeconds)
	}
	if !response.Allowed {
		t.Error("Allowed = false, want true")
	}
	if response.DenyReason != nil {
		t.Errorf("DenyReason = %v, want nil", *response.DenyReason)
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
	if got := response.ReportedUsedSeconds; got != 1200 {
		t.Errorf("ReportedUsedSeconds after lower report = %d, want 1200", got)
	}
	if got := store.Usage("test-host", "dani", "2024-06-03"); got != 1200 {
		t.Errorf("stored Usage = %d, want 1200", got)
	}
}

func TestStatusHandlerDailyLimitReached(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	rec := putStatus(t, server, statusBody(3600))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if response.Allowed {
		t.Error("Allowed = true, want false")
	}
	if response.DenyReason == nil || *response.DenyReason != "daily_limit_reached" {
		t.Errorf("DenyReason = %v, want daily_limit_reached", response.DenyReason)
	}
	if response.RemainingSeconds == nil || *response.RemainingSeconds != 0 {
		t.Errorf("RemainingSeconds = %v, want 0", response.RemainingSeconds)
	}
}

func TestStatusHandlerDayMismatch(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	rec := putStatus(t, server, statusBodyWithDay(100, "2024-06-04"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatusHandlerRequiresHostname(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	body := strings.Replace(statusBody(100), `"hostname": "test-host",`, "", 1)
	rec := putStatus(t, server, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", rec.Code, rec.Body.String())
	}
}

func TestStatusHandlerUnknownUser(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status/other", strings.NewReader(statusBody(100)))
	authorize(req)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStatusHandlerRejectsInvalidJSON(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/status/dani", strings.NewReader("{"))
	authorize(req)
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
	if response.Allowed {
		t.Error("Allowed = true, want false")
	}
	if response.DenyReason == nil || *response.DenyReason != "outside_window" {
		t.Errorf("DenyReason = %v, want outside_window", response.DenyReason)
	}
}

func TestAggregateUsageAcrossHostnames(t *testing.T) {
	server, _ := newTestServer(t, testNow())

	if rec := putStatusAs(t, server, "pc-one", statusBody(1000)); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rec.Code)
	}
	rec := putStatusAs(t, server, "pc-two", statusBody(500))
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode second status: %v", err)
	}
	if response.ReportedUsedSeconds != 500 {
		t.Errorf("ReportedUsedSeconds = %d, want 500", response.ReportedUsedSeconds)
	}
	if response.AggregateUsedSeconds != 1500 {
		t.Errorf("AggregateUsedSeconds = %d, want 1500", response.AggregateUsedSeconds)
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
	server := NewServer(config, store, testKey)
	server.now = func() time.Time { return now }
	return server, store
}

func authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+testKey)
}

func getListing(server *Server) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/enforcers", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func putStatus(t *testing.T, server *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	return putStatusAs(t, server, "test-host", body)
}

func putStatusAs(t *testing.T, server *Server, hostname, body string) *httptest.ResponseRecorder {
	t.Helper()
	if hostname != "test-host" {
		body = strings.Replace(body, `"hostname": "test-host"`, `"hostname": "`+hostname+`"`, 1)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/status/dani", strings.NewReader(body))
	authorize(req)
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
		"hostname": "test-host",
		"health": {"status": "ok", "last_error": null},
		"active": true,
		"locked": false,
		"session_count": 1,
		"usage": [{"day": "` + day + `", "used_seconds": ` + itoa(used) + `}],
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
