package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"timelord/internal/schedule"
)

// UsageSource reports the current daily usage of managed users.
type UsageSource interface {
	UsageSnapshot() []schedule.Usage
}

// UsageCollector exposes the remaining daily time for managed users.
type UsageCollector struct {
	source    UsageSource
	remaining *prometheus.Desc
}

// NewUsageCollector returns a collector that reads from source.
func NewUsageCollector(source UsageSource) *UsageCollector {
	return &UsageCollector{
		source: source,
		remaining: prometheus.NewDesc(
			"timelord_schedule_remaining_seconds",
			"Active seconds remaining in the user's current daily schedule. Users with no schedule report 0; users without a daily limit are omitted.",
			[]string{"user"},
			nil,
		),
	}
}

// Describe sends the metric descriptions to Prometheus.
func (c *UsageCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.remaining
}

// Collect sends the remaining time for each limited user.
func (c *UsageCollector) Collect(ch chan<- prometheus.Metric) {
	for _, usage := range c.source.UsageSnapshot() {
		if usage.HasPolicy && usage.Limit <= 0 {
			continue
		}

		var remaining time.Duration
		if usage.HasPolicy {
			remaining = usage.Limit - usage.Used
			if remaining < 0 {
				remaining = 0
			}
		}
		ch <- prometheus.MustNewConstMetric(c.remaining, prometheus.GaugeValue, remaining.Seconds(), usage.User)
	}
}
