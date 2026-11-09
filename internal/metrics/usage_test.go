package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"timelord/internal/schedule"
)

type fakeUsageSource struct {
	usage []schedule.Usage
}

func (f fakeUsageSource) UsageSnapshot() []schedule.Usage {
	return f.usage
}

func TestUsageCollector(t *testing.T) {
	source := fakeUsageSource{usage: []schedule.Usage{
		{User: "dani", Used: 15 * time.Minute, Limit: time.Hour, HasPolicy: true},
		{User: "alice", Used: 2 * time.Hour, Limit: time.Hour, HasPolicy: true},
		{User: "unlimited", Used: 2 * time.Hour, HasPolicy: true},
		{User: "no-policy"},
	}}

	expected := `
# HELP timelord_schedule_remaining_seconds Active seconds remaining in the user's current daily schedule. Users with no schedule report 0; users without a daily limit are omitted.
# TYPE timelord_schedule_remaining_seconds gauge
timelord_schedule_remaining_seconds{user="alice"} 0
timelord_schedule_remaining_seconds{user="dani"} 2700
timelord_schedule_remaining_seconds{user="no-policy"} 0
`
	if err := testutil.CollectAndCompare(NewUsageCollector(source), strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}
