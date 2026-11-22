package enforcer

import (
	"context"
	"testing"
	"time"

	"timelord/internal/notify"
	"timelord/internal/session"
)

func TestAgentNotifiesLimitThresholds(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent, notifier, _ := newNotifyAgent(t, &now, sessions)
	state := agent.states["dani"]
	const limit = int64(3600)

	state.aggregate = limit - 14*60
	agent.enforce(context.Background())
	state.aggregate = limit - 4*60
	agent.enforce(context.Background())
	state.aggregate = limit
	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{
		{notify.KindLimit, 15},
		{notify.KindLimit, 5},
		{notify.KindLimit, 0},
	})
}

func TestAgentSkipsLessUrgentThresholdsOnStartup(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent, notifier, _ := newNotifyAgent(t, &now, sessions)
	state := agent.states["dani"]
	const limit = int64(3600)

	state.aggregate = limit - 3*60
	agent.enforce(context.Background())
	agent.enforce(context.Background())
	state.aggregate = limit
	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{
		{notify.KindLimit, 5},
		{notify.KindLimit, 0},
	})
}

func TestAgentNotifiesAgainWhenLimitChanges(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent, notifier, _ := newNotifyAgent(t, &now, sessions)
	state := agent.states["dani"]

	state.aggregate = 3600 - 4*60
	agent.enforce(context.Background())
	state.policy.DailyLimitSeconds = 1800
	state.aggregate = 1800 - 4*60
	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{
		{notify.KindLimit, 5},
		{notify.KindLimit, 5},
	})
}

func TestAgentNotifiesWindowEnd(t *testing.T) {
	now := time.Date(2024, time.June, 3, 18, 52, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	agent, notifier, _ := newNotifyAgent(t, &now, sessions)

	agent.enforce(context.Background())
	now = time.Date(2024, time.June, 3, 18, 56, 0, 0, time.UTC)
	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{
		{notify.KindWindow, 10},
		{notify.KindWindow, 5},
	})
}

func TestAgentSkipsNoticesForIneligibleSessions(t *testing.T) {
	tests := []struct {
		name      string
		now       time.Time
		session   session.Session
		aggregate int64
	}{
		{
			name:      "locked",
			now:       time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
			session:   session.Session{ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true, Locked: true},
			aggregate: 3599,
		},
		{
			name:      "unmanaged",
			now:       time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
			session:   session.Session{ID: "1", User: "other", Seat: "seat0", Class: "user", Active: true},
			aggregate: 3599,
		},
		{
			name:      "outside window",
			now:       time.Date(2024, time.June, 3, 20, 0, 0, 0, time.UTC),
			session:   session.Session{ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true},
			aggregate: 3599,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := test.now
			sessions := &fakeSessions{sessions: []session.Session{test.session}}
			agent, notifier, _ := newNotifyAgent(t, &now, sessions)
			agent.states["dani"].aggregate = test.aggregate
			agent.enforce(context.Background())
			if len(notifier.notices) != 0 {
				t.Fatalf("notices = %v, want none", notifier.notices)
			}
		})
	}
}

func TestAgentNotifiesOneSessionPerUser(t *testing.T) {
	now := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{
		{ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true},
		{ID: "2", User: "dani", Seat: "seat0", Class: "user", Active: true},
	}}
	agent, notifier, _ := newNotifyAgent(t, &now, sessions)
	agent.states["dani"].aggregate = 3600 - 60

	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{{notify.KindLimit, 5}})
	if got := notifier.notices[0].Session.ID; got != "1" {
		t.Errorf("notified session = %s, want 1", got)
	}
}

func TestAgentDoesNotNotifyUnlimitedLimit(t *testing.T) {
	now := time.Date(2024, time.June, 3, 18, 55, 0, 0, time.UTC)
	sessions := &fakeSessions{sessions: []session.Session{{
		ID: "1", User: "dani", Seat: "seat0", Class: "user", Active: true,
	}}}
	client := &fakeClient{schedule: testSchedule(t, now), scheduleRevision: 42, statusRevision: 42}
	client.schedule.Policy.DailyLimitSeconds = 0
	agent := newTestAgent(t, &now, client, sessions)
	notifier := &fakeNotifier{}
	agent.notifier = notifier
	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}

	agent.enforce(context.Background())

	assertNotices(t, notifier, []noticeWant{{notify.KindWindow, 5}})
}

func TestWindowRemaining(t *testing.T) {
	schedule := testSchedule(t, time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC))
	tests := []struct {
		now  time.Time
		want time.Duration
		ok   bool
	}{
		{time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC), 9 * time.Hour, true},
		{time.Date(2024, time.June, 3, 18, 55, 0, 0, time.UTC), 5 * time.Minute, true},
		{time.Date(2024, time.June, 3, 19, 0, 0, 0, time.UTC), 0, false},
		{time.Date(2024, time.June, 3, 7, 59, 0, 0, time.UTC), 0, false},
	}
	for _, test := range tests {
		got, ok := windowRemaining(test.now, schedule.Policy, time.UTC)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("windowRemaining(%s) = %s, %t, want %s, %t", test.now, got, ok, test.want, test.ok)
		}
	}
}

type noticeWant struct {
	kind    notify.Kind
	minutes int
}

func assertNotices(t *testing.T, notifier *fakeNotifier, want []noticeWant) {
	t.Helper()
	if len(notifier.notices) != len(want) {
		t.Fatalf("notices = %v, want %d", notifier.notices, len(want))
	}
	for i, expected := range want {
		got := notifier.notices[i].Notice
		if got.Kind != expected.kind || got.Minutes != expected.minutes {
			t.Errorf("notice %d = %+v, want %+v", i, got, expected)
		}
	}
}

func newNotifyAgent(t *testing.T, now *time.Time, sessions *fakeSessions) (*Agent, *fakeNotifier, *fakeClient) {
	t.Helper()
	client := &fakeClient{schedule: testSchedule(t, *now), scheduleRevision: 42, statusRevision: 42}
	agent := newTestAgent(t, now, client, sessions)
	notifier := &fakeNotifier{}
	agent.notifier = notifier
	if err := agent.fetchSchedules(context.Background()); err != nil {
		t.Fatalf("fetchSchedules() error = %v", err)
	}
	return agent, notifier, client
}

type notified struct {
	Session session.Session
	Notice  notify.Notice
}

type fakeNotifier struct {
	notices []notified
	err     error
}

func (f *fakeNotifier) Notify(_ context.Context, sess session.Session, notice notify.Notice) error {
	f.notices = append(f.notices, notified{Session: sess, Notice: notice})
	return f.err
}
