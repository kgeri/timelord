package schedule

import (
	"testing"
	"time"
)

func TestStoreReplaceFiltersManagedUsers(t *testing.T) {
	store := NewStore([]string{"dani", "alice"})
	store.Replace(&Config{Users: []Policy{
		{User: "alice", DailyLimitMinutes: 30},
		{User: "someone-else", DailyLimitMinutes: 10},
	}})

	if _, ok := store.Policy("alice"); !ok {
		t.Error("alice policy is missing")
	}
	if _, ok := store.Policy("dani"); ok {
		t.Error("dani has a policy, want none")
	}
	if _, ok := store.Policy("someone-else"); ok {
		t.Error("unmanaged user has a policy")
	}
}

func TestStoreAllowed(t *testing.T) {
	store := NewStore([]string{"dani"})
	store.Replace(&Config{Users: []Policy{{
		User:              "dani",
		DailyLimitMinutes: 60,
		Windows:           []Window{{Day: Monday, Start: 8 * 60, End: 19 * 60}},
	}}})

	monday := time.Date(2024, time.June, 3, 10, 0, 0, 0, time.Local)
	if !store.Allowed("dani", monday, 0) {
		t.Error("Allowed() = false, want true")
	}
	if store.Allowed("dani", monday, time.Hour) {
		t.Error("Allowed() at daily limit = true, want false")
	}
	if store.Allowed("dani", monday.AddDate(0, 0, 1), 0) {
		t.Error("Allowed() on a day without a window = true, want false")
	}
	if store.Allowed("alice", monday, 0) {
		t.Error("Allowed() for an unmanaged user = true, want false")
	}
}
