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
	cache.Set([]process.Process{
		{User: "alice", Name: "bash", Executable: "/bin/bash"},
		{User: "alice", Name: "bash", Executable: "/usr/local/bin/bash"},
		{User: "alice", Name: "chrome", Executable: "/opt/chrome"},
	})

	expected := `
# HELP timelord_process Number of running processes per user and name.
# TYPE timelord_process gauge
timelord_process{name="bash",user="alice"} 2
timelord_process{name="chrome",user="alice"} 1
`
	if err := testutil.CollectAndCompare(NewCollector(cache), strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

func TestServerServesMetrics(t *testing.T) {
	cache := process.NewCache()
	cache.Set([]process.Process{{User: "alice", Name: "bash", Executable: "/bin/bash"}})

	server := NewServer(":0", cache)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "timelord_process") {
		t.Errorf("response does not contain timelord_process:\n%s", rec.Body.String())
	}
}
