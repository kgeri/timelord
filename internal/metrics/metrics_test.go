package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"timelord/internal/process"
)

func TestCollectorAggregatesByUserAndName(t *testing.T) {
	cache := process.NewCache()

	// The first sample sets the CPU baseline.
	cache.Set([]process.Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp, MemoryBytes: 100, CPUSeconds: 10},
		{PID: 1002, StartTime: 1, User: "alice", Name: "bash", Executable: "/usr/local/bin/bash", Scope: process.ScopeApp, MemoryBytes: 200, CPUSeconds: 20},
	})
	// The second sample adds CPU time.
	cache.Set([]process.Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp, MemoryBytes: 120, CPUSeconds: 11},
		{PID: 1002, StartTime: 1, User: "alice", Name: "bash", Executable: "/usr/local/bin/bash", Scope: process.ScopeApp, MemoryBytes: 250, CPUSeconds: 23},
		{PID: 1003, StartTime: 1, User: "alice", Name: "chrome", Executable: "/opt/chrome", Scope: process.ScopeApp, MemoryBytes: 300, CPUSeconds: 5},
	})

	expected := `
# HELP timelord_process_cpu_seconds_total CPU time that TimeLord observed for processes per user, name, and scope.
# TYPE timelord_process_cpu_seconds_total counter
timelord_process_cpu_seconds_total{name="bash",scope="app",user="alice"} 4
timelord_process_cpu_seconds_total{name="chrome",scope="app",user="alice"} 0
# HELP timelord_process_instances Number of running processes per user, name, and scope.
# TYPE timelord_process_instances gauge
timelord_process_instances{name="bash",scope="app",user="alice"} 2
timelord_process_instances{name="chrome",scope="app",user="alice"} 1
# HELP timelord_process_memory_rss_bytes Resident memory of running processes per user, name, and scope.
# TYPE timelord_process_memory_rss_bytes gauge
timelord_process_memory_rss_bytes{name="bash",scope="app",user="alice"} 370
timelord_process_memory_rss_bytes{name="chrome",scope="app",user="alice"} 300
`
	if err := testutil.CollectAndCompare(NewCollector(cache), strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorSeparatesScopes(t *testing.T) {
	cache := process.NewCache()
	cache.Set([]process.Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp, MemoryBytes: 100},
		{PID: 1002, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeSystem, MemoryBytes: 200},
	})

	expected := `
# HELP timelord_process_instances Number of running processes per user, name, and scope.
# TYPE timelord_process_instances gauge
timelord_process_instances{name="bash",scope="app",user="alice"} 1
timelord_process_instances{name="bash",scope="system",user="alice"} 1
`
	if err := testutil.CollectAndCompare(NewCollector(cache), strings.NewReader(expected), "timelord_process_instances"); err != nil {
		t.Fatal(err)
	}
}

func TestServerServesMetrics(t *testing.T) {
	cache := process.NewCache()
	cache.Set([]process.Process{
		{PID: 1001, StartTime: 1, User: "alice", Name: "bash", Executable: "/bin/bash", Scope: process.ScopeApp},
	})

	server := NewServer(":0", cache)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "timelord_process_instances") {
		t.Errorf("response does not contain timelord_process_instances:\n%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `scope="app"`) {
		t.Errorf("response does not contain the scope label:\n%s", rec.Body.String())
	}
}
