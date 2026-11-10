package enforce

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"timelord/internal/schedule"
)

type fakeSessionSource struct {
	sessions   []Session
	err        error
	terminated []string
}

func (f *fakeSessionSource) Sessions(context.Context) ([]Session, error) {
	return f.sessions, f.err
}

func (f *fakeSessionSource) TerminateSession(_ context.Context, id string) error {
	f.terminated = append(f.terminated, id)
	return nil
}

func TestEngineTracksActiveSessionTime(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.interval = time.Hour

	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }

	engine.tick(context.Background())
	now = now.Add(30 * time.Minute)
	engine.tick(context.Background())

	usage := engine.UsageSnapshot()
	if len(usage) != 1 {
		t.Fatalf("len(UsageSnapshot()) = %d, want 1", len(usage))
	}
	if usage[0].Used != 30*time.Minute {
		t.Errorf("Used = %v, want 30m", usage[0].Used)
	}
	if len(source.terminated) != 0 {
		t.Errorf("terminated = %v, want none", source.terminated)
	}
}

func TestEngineDoesNotCountLockedSession(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true, Locked: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.interval = time.Hour

	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }

	engine.tick(context.Background())
	now = now.Add(time.Hour)
	engine.tick(context.Background())

	usage := engine.UsageSnapshot()
	if usage[0].Used != 0 {
		t.Errorf("Used = %v, want 0", usage[0].Used)
	}
	if len(source.terminated) != 0 {
		t.Errorf("terminated = %v, want none", source.terminated)
	}
}

func TestEngineIgnoresRemoteSessions(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Class: "user", Remote: true, Active: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.interval = time.Hour

	now := time.Date(2024, time.June, 3, 20, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }

	engine.tick(context.Background())
	now = now.Add(time.Hour)
	engine.tick(context.Background())

	usage := engine.UsageSnapshot()
	if usage[0].Used != 0 {
		t.Errorf("Used = %v, want 0", usage[0].Used)
	}
	if len(source.terminated) != 0 {
		t.Errorf("terminated = %v, want none", source.terminated)
	}
}

func TestEngineTerminatesOutsideWindow(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.now = func() time.Time {
		return time.Date(2024, time.June, 3, 20, 0, 0, 0, time.Local)
	}

	engine.tick(context.Background())

	if len(source.terminated) != 1 || source.terminated[0] != "1" {
		t.Fatalf("terminated = %v, want [1]", source.terminated)
	}
}

func TestEngineTerminatesAtDailyLimit(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.interval = time.Hour

	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }

	engine.tick(context.Background())
	now = now.Add(time.Hour)
	engine.tick(context.Background())

	if len(source.terminated) != 1 || source.terminated[0] != "1" {
		t.Fatalf("terminated = %v, want [1]", source.terminated)
	}
}

func TestEngineTerminatesWithoutPolicy(t *testing.T) {
	store := schedule.NewStore([]string{"dani"})
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true, Locked: true,
	}}}
	engine := NewEngine(source, store, nil)

	engine.tick(context.Background())

	if len(source.terminated) != 1 || source.terminated[0] != "1" {
		t.Fatalf("terminated = %v, want [1]", source.terminated)
	}
}

func TestEngineAuditsSessionTransitions(t *testing.T) {
	store := testStore(60)
	source := &fakeSessionSource{sessions: []Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	engine := NewEngine(source, store, nil)
	engine.interval = time.Hour

	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	engine.now = func() time.Time { return now }

	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })

	// The first observation is only a baseline.
	engine.tick(context.Background())
	if strings.Contains(output.String(), "AUDIT:") {
		t.Fatalf("first tick logged audit events: %s", output.String())
	}

	source.sessions[0].Locked = true
	engine.tick(context.Background())
	if !strings.Contains(output.String(), "AUDIT: lock user=\"dani\" session=1") {
		t.Errorf("missing lock audit event:\n%s", output.String())
	}

	output.Reset()
	source.sessions[0].Locked = false
	engine.tick(context.Background())
	if !strings.Contains(output.String(), "AUDIT: unlock user=\"dani\" session=1") {
		t.Errorf("missing unlock audit event:\n%s", output.String())
	}

	output.Reset()
	source.sessions = []Session{{
		ID: "2", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}
	engine.tick(context.Background())
	if !strings.Contains(output.String(), "AUDIT: login user=\"dani\" session=2") {
		t.Errorf("missing login audit event:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "AUDIT: logout user=\"dani\" session=1") {
		t.Errorf("missing logout audit event:\n%s", output.String())
	}
}

func TestEngineUsageSnapshotIncludesManagedUsersWithoutPolicy(t *testing.T) {
	store := schedule.NewStore([]string{"dani", "alice"})
	store.Replace(&schedule.Config{Users: []schedule.Policy{{
		User:              "dani",
		DailyLimitMinutes: 60,
	}}})
	engine := NewEngine(&fakeSessionSource{}, store, nil)
	engine.now = func() time.Time {
		return time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	}

	usage := engine.UsageSnapshot()
	if len(usage) != 2 {
		t.Fatalf("len(UsageSnapshot()) = %d, want 2", len(usage))
	}
	if usage[0].User != "dani" || !usage[0].HasPolicy || usage[0].Limit != time.Hour {
		t.Errorf("dani usage = %+v", usage[0])
	}
	if usage[1].User != "alice" || usage[1].HasPolicy {
		t.Errorf("alice usage = %+v", usage[1])
	}
}

func TestEngineRolloverResetsUsage(t *testing.T) {
	engine := NewEngine(&fakeSessionSource{}, schedule.NewStore([]string{"dani"}), nil)
	engine.used["dani"] = time.Hour
	engine.date = "2024-06-03"

	engine.rolloverLocked(time.Date(2024, time.June, 4, 0, 0, 0, 0, time.Local))

	if got := engine.used["dani"]; got != 0 {
		t.Errorf("used after rollover = %v, want 0", got)
	}
}

func testStore(limitMinutes int) *schedule.Store {
	store := schedule.NewStore([]string{"dani"})
	store.Replace(&schedule.Config{Users: []schedule.Policy{{
		User:              "dani",
		DailyLimitMinutes: limitMinutes,
		Windows: []schedule.Window{{
			Day:   schedule.Monday,
			Start: 8 * 60,
			End:   19 * 60,
		}},
	}}})
	return store
}
