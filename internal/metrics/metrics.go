package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"timelord/internal/process"
)

// Collector exposes the process cache as the timelord_process metric.
type Collector struct {
	cache *process.Cache
	desc  *prometheus.Desc
}

// NewCollector returns a collector that reads from cache.
func NewCollector(cache *process.Cache) *Collector {
	return &Collector{
		cache: cache,
		desc: prometheus.NewDesc(
			"timelord_process",
			"Number of running processes per user and name.",
			[]string{"user", "name"},
			nil,
		),
	}
}

// Describe sends the metric description to Prometheus.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

// Collect sends one metric for each user and name pair.
// It adds the counts of entries that share the same user and name.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	counts := make(map[userName]int)
	for _, entry := range c.cache.Snapshot() {
		counts[userName{user: entry.User, name: entry.Name}] += entry.Count
	}

	for key, count := range counts {
		ch <- prometheus.MustNewConstMetric(
			c.desc,
			prometheus.GaugeValue,
			float64(count),
			key.user,
			key.name,
		)
	}
}

// userName identifies one metric series.
type userName struct {
	user string
	name string
}
