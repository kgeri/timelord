package enforcer

import (
	"context"
	"testing"
	"time"

	"timelord/internal/policy"
	"timelord/internal/process"
	"timelord/internal/session"
	"timelord/internal/timetower"
)

func TestAgentRecordsUsageAndReportsStatus(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(now), scheduleRevision: 42, statusRevision: 42}
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
	if len(request.Usage) != 1 || request.Usage[0].UsedSeconds != 15 {
		t.Errorf("Usage = %+v, want current day 15 seconds", request.Usage)
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
	remaining := int64(3100)
	client := &fakeClient{
		schedule:         testSchedule(now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse: &timetower.StatusResponse{
			ServerTime:           now,
			ServerDay:            "2024-06-03",
			User:                 "dani",
			CurrentDay:           "2024-06-03",
			ReportedUsedSeconds:  100,
			AggregateUsedSeconds: 500,
			DailyLimitSeconds:    3600,
			RemainingSeconds:     &remaining,
			InsideWindow:         true,
			Allowed:              true,
			AcknowledgedDays:     []string{"2024-06-03"},
		},
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
	remaining := int64(2600)
	client := &fakeClient{
		schedule:         testSchedule(now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse: &timetower.StatusResponse{
			ServerTime:           now,
			ServerDay:            "2024-06-03",
			User:                 "dani",
			CurrentDay:           "2024-06-03",
			ReportedUsedSeconds:  500,
			AggregateUsedSeconds: 1000,
			DailyLimitSeconds:    3600,
			RemainingSeconds:     &remaining,
			InsideWindow:         true,
			Allowed:              true,
			AcknowledgedDays:     []string{"2024-06-03"},
		},
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
	client := &fakeClient{schedule: testSchedule(now), scheduleRevision: 42, statusRevision: 43}
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
	client := &fakeClient{schedule: testSchedule(now), scheduleRevision: 42, statusRevision: 42}
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

func TestAgentDayRolloverMovesUsageToPending(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(now), scheduleRevision: 42, statusRevision: 42}
	agent := newTestAgent(t, &now, client, &fakeSessions{})

	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	state := agent.states["dani"]
	state.local = 100
	state.reported = 100
	state.aggregate = 500

	agent.rolloverDay("2024-06-04")

	if agent.day != "2024-06-04" {
		t.Errorf("day = %q, want 2024-06-04", agent.day)
	}
	if state.local != 0 || state.reported != 0 || state.aggregate != 0 {
		t.Errorf("state after rollover = %+v, want zeros", state)
	}
	if state.pending["2024-06-03"] != 100 {
		t.Errorf("pending usage = %d, want 100", state.pending["2024-06-03"])
	}

	agent.rolloverDay("2024-06-11")
	if _, ok := state.pending["2024-06-03"]; ok {
		t.Error("old pending day was not pruned")
	}
}

func TestAgentAdoptsServerDayOnStatus(t *testing.T) {
	now := time.Date(2024, time.June, 3, 23, 59, 0, 0, time.UTC)
	remaining := int64(3580)
	client := &fakeClient{
		schedule:         testSchedule(now),
		scheduleRevision: 42,
		statusRevision:   42,
		statusResponse: &timetower.StatusResponse{
			ServerTime:           now.Add(2 * time.Minute),
			ServerDay:            "2024-06-04",
			User:                 "dani",
			CurrentDay:           "2024-06-04",
			ReportedUsedSeconds:  20,
			AggregateUsedSeconds: 20,
			DailyLimitSeconds:    3600,
			RemainingSeconds:     &remaining,
			InsideWindow:         false,
			Allowed:              false,
			AcknowledgedDays:     []string{"2024-06-04"},
		},
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
	if state.pending["2024-06-03"] != 100 {
		t.Errorf("pending usage = %d, want 100", state.pending["2024-06-03"])
	}
	if state.local != 20 {
		t.Errorf("local usage = %d, want 20", state.local)
	}
}

func TestAgentOfflineGraceDenies(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	client := &fakeClient{schedule: testSchedule(now), scheduleRevision: 42, statusRevision: 42}
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
		Users:     []string{"dani"},
		Now:       func() time.Time { return *now },
	})
}

func testSchedule(now time.Time) *timetower.Schedule {
	remaining := int64(3600)
	return &timetower.Schedule{
		SchemaVersion:         1,
		ServerTime:            now,
		Timezone:              "UTC",
		StatusIntervalSeconds: 60,
		OfflineGraceSeconds:   3600,
		User:                  "dani",
		Policy: &timetower.Policy{
			Enabled:           true,
			DailyLimitSeconds: 3600,
			Windows: []policy.Window{{
				Day: policy.Monday, Start: 8 * 60, End: 19 * 60,
			}},
		},
		Usage: timetower.UsageState{
			Day:              now.Format(dateLayout),
			UsedSeconds:      0,
			RemainingSeconds: &remaining,
		},
	}
}

type fakeClient struct {
	schedule         *timetower.Schedule
	scheduleRevision int64
	scheduleErr      error
	statusResponse   *timetower.StatusResponse
	statusRevision   int64
	statusErr        error
	fetchCount       int
	statusRequests   []timetower.StatusRequest
}

func (f *fakeClient) FetchSchedule(context.Context, string) (*timetower.Schedule, int64, error) {
	f.fetchCount++
	if f.scheduleErr != nil {
		return nil, 0, f.scheduleErr
	}
	return f.schedule, f.scheduleRevision, nil
}

func (f *fakeClient) SendStatus(_ context.Context, _ string, request timetower.StatusRequest) (*timetower.StatusResponse, int64, error) {
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
