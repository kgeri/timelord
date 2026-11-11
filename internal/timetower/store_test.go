package timetower

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"timelord/internal/policy"
)

func TestStoreUsageIsMonotonicAndAggregates(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	store.RecordUsage("pc-one", "dani", "2024-06-03", 100)
	store.RecordUsage("pc-one", "dani", "2024-06-03", 50)
	if got := store.Usage("pc-one", "dani", "2024-06-03"); got != 100 {
		t.Errorf("Usage() = %d, want 100", got)
	}

	store.RecordUsage("pc-two", "dani", "2024-06-03", 25)
	if got := store.AggregateUsage("dani", "2024-06-03"); got != 125 {
		t.Errorf("AggregateUsage() = %d, want 125", got)
	}
	if got := store.AggregateUsage("dani", "2024-06-04"); got != 0 {
		t.Errorf("AggregateUsage(other day) = %d, want 0", got)
	}
}

func TestStoreOverride(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	if _, ok := store.Override("dani", "2024-06-03"); ok {
		t.Fatal("Override() = set, want unset")
	}

	start := policy.Clock(10 * 60)
	end := policy.Clock(18 * 60)
	limit := int64(15)
	store.SetOverride("dani", "2024-06-02", DayOverride{Start: &start})
	store.SetOverride("dani", "2024-06-03", DayOverride{
		Start:        &start,
		End:          &end,
		LimitMinutes: &limit,
	})
	override, ok := store.Override("dani", "2024-06-03")
	if !ok {
		t.Fatal("Override() = unset, want set")
	}
	if override.Start == nil || *override.Start != start ||
		override.End == nil || *override.End != end ||
		override.LimitMinutes == nil || *override.LimitMinutes != limit {
		t.Errorf("Override() = %+v, want start 10:00, end 18:00, limit 15", override)
	}
	if _, ok := store.Override("dani", "2024-06-02"); ok {
		t.Error("older override was not pruned on set")
	}

	before := store.OverridesRevision("2024-06-03")
	limit = 30
	store.SetOverride("dani", "2024-06-03", DayOverride{
		Start:        &start,
		End:          &end,
		LimitMinutes: &limit,
	})
	if after := store.OverridesRevision("2024-06-03"); after == before {
		t.Error("OverridesRevision() did not change with the override")
	}

	store.ClearOverride("dani", "2024-06-03")
	if _, ok := store.Override("dani", "2024-06-03"); ok {
		t.Error("Override() = set after clear, want unset")
	}
}

func TestStorePruneUsage(t *testing.T) {
	store, err := NewStore("")
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.RecordUsage("pc-one", "dani", "2024-06-01", 10)
	store.RecordUsage("pc-one", "dani", "2024-06-03", 20)

	store.PruneUsage("2024-06-02")

	if got := store.Usage("pc-one", "dani", "2024-06-01"); got != 0 {
		t.Errorf("old usage = %d, want 0", got)
	}
	if got := store.Usage("pc-one", "dani", "2024-06-03"); got != 20 {
		t.Errorf("current usage = %d, want 20", got)
	}
}

func TestStorePersistenceWritesCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.csv")
	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	store.RecordUsage("pc-one", "dani", "2024-06-03", 100)
	store.RecordUsage("pc-two", "dani", "2024-06-03", 50)
	store.SetStatus("pc-one", "dani", StatusRecord{
		Hostname:   "pc-one",
		User:       "dani",
		ReceivedAt: time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
		Request: StatusRequest{
			ObservedAt:      time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
			EnforcerVersion: "0.2.0",
			Hostname:        "pc-one",
			UsageSeconds:    0,
			Processes:       []ProcessStat{},
		},
	})
	if err := store.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"date,user,hostname,used_seconds\n",
		"2024-06-03,dani,pc-one,100\n",
		"2024-06-03,dani,pc-two,50\n",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("CSV missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "enforcer_version") || strings.Contains(content, "processes") {
		t.Errorf("CSV contains status data:\n%s", content)
	}

	loaded, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore(load) error = %v", err)
	}
	if got := loaded.AggregateUsage("dani", "2024-06-03"); got != 150 {
		t.Errorf("loaded AggregateUsage() = %d, want 150", got)
	}
	if records := loaded.StatusRecords(); len(records) != 0 {
		t.Errorf("loaded StatusRecords() = %d, want 0", len(records))
	}
}
