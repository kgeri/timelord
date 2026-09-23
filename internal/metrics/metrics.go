package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"timelord/internal/process"
)

// Collector exposes the process cache as Prometheus metrics.
type Collector struct {
	cache      *process.Cache
	instances  *prometheus.Desc
	memoryRSS  *prometheus.Desc
	cpuSeconds *prometheus.Desc
}

// NewCollector returns a collector that reads from cache.
func NewCollector(cache *process.Cache) *Collector {
	return &Collector{
		cache: cache,
		instances: prometheus.NewDesc(
			"timelord_process_instances",
			"Number of running processes per user and name.",
			[]string{"user", "name"},
			nil,
		),
		memoryRSS: prometheus.NewDesc(
			"timelord_process_memory_rss_bytes",
			"Resident memory of running processes per user and name.",
			[]string{"user", "name"},
			nil,
		),
		cpuSeconds: prometheus.NewDesc(
			"timelord_process_cpu_seconds_total",
			"CPU time that TimeLord observed for processes per user and name.",
			[]string{"user", "name"},
			nil,
		),
	}
}

// Describe sends the metric descriptions to Prometheus.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.instances
	ch <- c.memoryRSS
	ch <- c.cpuSeconds
}

// series is the aggregate of all entries that share a user and name.
type series struct {
	instances   int
	memoryBytes uint64
	cpuSeconds  float64
}

// Collect sends one metric for each user and name pair.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	byName := make(map[userName]series)
	for _, entry := range c.cache.Snapshot() {
		key := userName{user: entry.User, name: entry.Name}
		s := byName[key]
		s.instances += entry.Count
		s.memoryBytes += entry.MemoryBytes
		s.cpuSeconds += entry.CPUSeconds
		byName[key] = s
	}

	for key, s := range byName {
		ch <- prometheus.MustNewConstMetric(c.instances, prometheus.GaugeValue, float64(s.instances), key.user, key.name)
		ch <- prometheus.MustNewConstMetric(c.memoryRSS, prometheus.GaugeValue, float64(s.memoryBytes), key.user, key.name)
		ch <- prometheus.MustNewConstMetric(c.cpuSeconds, prometheus.CounterValue, s.cpuSeconds, key.user, key.name)
	}
}

// userName identifies one metric series.
type userName struct {
	user string
	name string
}
