package enforcer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"timelord/internal/process"
	"timelord/internal/session"
	"timelord/internal/timetower"
)

func TestAgentRecordsUsageAndReportsStatus(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent := newTestAgent(t, &now, client, sessions)

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	agent.enforce(context.Background())
	now = now.Add(DefaultInterval)
	agent.enforce(context.Background())

	state := agent.states["dani"]
	if state.local != 15 {
		t.Fatalf("local usage = %d, want 15", state.local)
	}

	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if len(client.statusRequests) != 1 {
		t.Fatalf("status requests = %d, want 1", len(client.statusRequests))
	}
	request := client.statusRequests[0]
	if request.EnforcerVersion != "test" || request.Hostname != "test-host" {
		t.Errorf("request = %+v, want test version and hostname", request)
	}
	if !request.Active {
		t.Errorf("Active = false, want true")
	}
	if request.UsageSeconds != 15 {
		t.Errorf("UsageSeconds = %d, want 15", request.UsageSeconds)
	}
	if len(request.Processes) != 1 {
		t.Fatalf("len(Processes) = %d, want 1", len(request.Processes))
	}
	if request.Processes[0].Name != "firefox" || request.Processes[0].Scope != "app" {
		t.Errorf("process = %+v, want firefox/app", request.Processes[0])
	}
}

func TestAgentAdoptsServerUsage(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{
		schedule:         testSchedule(t, now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse:   testStatusResponse(t, "status", "response.json"),
	}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}

	state := agent.states["dani"]
	if state.local != 100 {
		t.Errorf("local usage = %d, want 100", state.local)
	}
	if state.reported != 100 {
		t.Errorf("reported usage = %d, want 100", state.reported)
	}
	if state.aggregate != 500 {
		t.Errorf("aggregate usage = %d, want 500", state.aggregate)
	}
}

func TestAgentAddsLocalUsageToOlderServerCounter(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{
		schedule:         testSchedule(t, now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse:   testStatusResponse(t, "status", "response-older-counter.json"),
	}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	state := agent.states["dani"]
	state.local = 15

	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if state.local != 515 {
		t.Errorf("local usage = %d, want 515", state.local)
	}
	if state.reported != 500 {
		t.Errorf("reported usage = %d, want 500", state.reported)
	}
	if state.aggregate != 1000 {
		t.Errorf("aggregate usage = %d, want 1000", state.aggregate)
	}
}

func TestAgentRefreshesSchedulesWhenRevisionChanges(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 43}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	client.scheduleRevision = 43
	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if client.fetchCount != 2 {
		t.Errorf("fetch count = %d, want 2", client.fetchCount)
	}
	if agent.revision != 43 {
		t.Errorf("revision = %d, want 43", agent.revision)
	}
}

func TestAgentTerminatesDeniedSession(t *testing.T) {
	now := time.Date(2024, time.June, 3, 20, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent := newTestAgent(t, &now, client, sessions)

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	agent.enforce(context.Background())

	if len(sessions.terminated) != 1 || sessions.terminated[0] != "1" {
		t.Fatalf("terminated = %v, want session 1", sessions.terminated)
	}
}

func TestAgentDoesNotTerminateInactiveOrLockedSessions(t *testing.T) {
	now := time.Date(2024, time.June, 3, 20, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	sessions := &fakeSessions{sessions: []session.Session{
		{ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: false},
		{ID: "2", User: "dani", Seat: "seat0", Class: "user", Active: true, Locked: true},
		{ID: "3", User: "dani", Seat: "seat0", Class: "greeter", Active: true},
	}}
	agent := newTestAgent(t, &now, client, sessions)

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	agent.enforce(context.Background())

	if len(sessions.terminated) != 0 {
		t.Fatalf("terminated = %v, want no sessions", sessions.terminated)
	}
}

func TestAgentMapsLocalUserToServerAccount(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "kissd", Seat: "Console", Class: "user", Active: true,
	}}}

	cache := process.NewCache()
	cache.Set([]process.Process{{
		PID: 1, User: "kissd", Name: "game", Executable: `C:\\Games\\game.exe`,
		Scope: process.ScopeApp, MemoryBytes: 100,
	}})
	agent := New(Config{
		Client: client, Sessions: sessions, Processes: cache,
		Version: "test", Hostname: "test-host",
		Users: []User{{Local: "kissd", Server: "dani"}},
		Now:   func() time.Time { return now },
	})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	if len(client.fetchUsers) != 1 || client.fetchUsers[0] != "dani" {
		t.Fatalf("fetched users = %v, want [dani]", client.fetchUsers)
	}
	if agent.states["dani"] == nil || agent.states["kissd"] != nil {
		t.Fatalf("states = %v, want only dani", agent.states)
	}

	agent.enforce(context.Background())
	now = now.Add(DefaultInterval)
	agent.enforce(context.Background())
	if got := agent.states["dani"].local; got != 15 {
		t.Fatalf("local usage = %d, want 15", got)
	}

	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if len(client.statusUsers) != 1 || client.statusUsers[0] != "dani" {
		t.Fatalf("status users = %v, want [dani]", client.statusUsers)
	}
	if !client.statusRequests[0].Active || client.statusRequests[0].UsageSeconds != 15 {
		t.Errorf("status request = %+v, want active with 15 seconds", client.statusRequests[0])
	}
}

func TestAgentAggregatesMultipleLocalAccountsForOneServerAccount(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	sessions := &fakeSessions{sessions: []session.Session{
		{ID: "1", User: "kid-one", Seat: "seat0", Class: "user", Active: true},
		{ID: "2", User: "kid-two", Seat: "seat0", Class: "user", Active: true},
	}}
	agent := New(Config{
		Client: client, Sessions: sessions, Processes: process.NewCache(),
		Version: "test", Hostname: "test-host",
		Users: []User{
			{Local: "kid-one", Server: "dani"},
			{Local: "kid-two", Server: "dani"},
		},
		Now: func() time.Time { return now },
	})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	if len(client.fetchUsers) != 1 || client.fetchUsers[0] != "dani" {
		t.Fatalf("fetched users = %v, want [dani]", client.fetchUsers)
	}

	agent.enforce(context.Background())
	now = now.Add(DefaultInterval)
	agent.enforce(context.Background())
	if got := agent.states["dani"].local; got != 15 {
		t.Fatalf("local usage = %d, want 15 (counted once)", got)
	}

	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if len(client.statusUsers) != 1 || client.statusUsers[0] != "dani" {
		t.Fatalf("status users = %v, want [dani]", client.statusUsers)
	}
	if got := client.statusRequests[0].SessionCount; got != 2 {
		t.Errorf("session count = %d, want 2", got)
	}
}

func TestAgentDayChangeResetsUsage(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	state := agent.states["dani"]
	state.local = 100
	state.reported = 100
	state.aggregate = 500

	agent.setDay("2024-06-03")
	if state.local != 100 || state.reported != 100 || state.aggregate != 500 {
		t.Errorf("state after same-day update = %+v, want unchanged", state)
	}

	agent.setDay("2024-06-04")
	if agent.day != "2024-06-04" {
		t.Errorf("day = %q, want 2024-06-04", agent.day)
	}
	if state.local != 0 || state.reported != 0 || state.aggregate != 0 {
		t.Errorf("state after day change = %+v, want zeros", state)
	}
}

func TestAgentAdoptsServerDayOnStatus(t *testing.T) {
	now := time.Date(2024, time.June, 3, 23, 59, 0, 0, time.UTC)
	client := &fakeClient{
		schedule:         testSchedule(t, now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse:   testStatusResponse(t, "status", "response-day-rollover.json"),
	}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	state := agent.states["dani"]
	state.local = 100

	if err := agent.reportStatuses(context.Background()); err != nil {
		t.Fatalf("reportStatuses() error = %v", err)
	}
	if agent.day != "2024-06-04" {
		t.Errorf("day = %q, want 2024-06-04", agent.day)
	}
	if state.local != 20 {
		t.Errorf("local usage = %d, want 20", state.local)
	}
}

func TestAgentOfflineGraceDenies(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	state := agent.states["dani"]
	if !agent.allowed(state, now) {
		t.Fatal("allowed before grace = false, want true")
	}

	now = now.Add(2 * time.Hour)
	if agent.allowed(state, now) {
		t.Fatal("allowed after grace = true, want false")
	}
}

func newTestAgent(t *testing.T, now *time.Time, client Client, sessions session.SessionSource) *Agent {
	t.Helper()
	cache := process.NewCache()
	cache.Set([]process.Process{{
		PID:         1,
		User:        "dani",
		Name:        "firefox",
		Executable:  "/usr/bin/firefox",
		Scope:       process.ScopeApp,
		MemoryBytes: 100,
	}})
	return New(Config{
		Client:    client,
		Sessions:  sessions,
		Processes: cache,
		Version:   "test",
		Hostname:  "test-host",
		Users:     []User{{Local: "dani", Server: "dani"}},
		Now:       func() time.Time { return *now },
	})
}

func testSchedule(t *testing.T, now time.Time) *timetower.Schedule {
	t.Helper()
	var schedule timetower.Schedule
	if err := json.Unmarshal(readTestdata(t, "schedule.json"), &schedule); err != nil {
		t.Fatalf("decode schedule testdata: %v", err)
	}
	schedule.ServerTime = now
	schedule.Usage.Day = now.Format(dateLayout)
	return &schedule
}

func testStatusResponse(t *testing.T, parts ...string) *timetower.StatusResponse {
	t.Helper()
	var response timetower.StatusResponse
	if err := json.Unmarshal(readTestdata(t, parts...), &response); err != nil {
		t.Fatalf("decode status response testdata: %v", err)
	}
	return &response
}

type fakeClient struct {
	schedule         *timetower.Schedule
	scheduleRevision int64
	scheduleErr      error
	statusResponse   *timetower.StatusResponse
	statusRevision   int64
	statusErr        error
	fetchCount       int
	fetchUsers       []string
	statusUsers      []string
	statusRequests   []timetower.StatusRequest
}

func (f *fakeClient) FetchSchedule(_ context.Context, user string) (*timetower.Schedule, int64, error) {
	f.fetchCount++
	f.fetchUsers = append(f.fetchUsers, user)
	if f.scheduleErr != nil {
		return nil, 0, f.scheduleErr
	}
	return f.schedule, f.scheduleRevision, nil
}

func (f *fakeClient) SendStatus(_ context.Context, user string, request timetower.StatusRequest) (*timetower.StatusResponse, int64, error) {
	f.statusUsers = append(f.statusUsers, user)
	f.statusRequests = append(f.statusRequests, request)
	if f.statusErr != nil {
		return nil, 0, f.statusErr
	}
	if f.statusResponse != nil {
		return f.statusResponse, f.statusRevision, nil
	}
	return &timetower.StatusResponse{}, f.statusRevision, nil
}

type fakeSessions struct {
	sessions   []session.Session
	err        error
	terminated []string
}

func (f *fakeSessions) Sessions(context.Context) ([]session.Session, error) {
	return f.sessions, f.err
}

func (f *fakeSessions) TerminateSession(_ context.Context, id string) error {
	f.terminated = append(f.terminated, id)
	return nil
}
