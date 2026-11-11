package timetower

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientFetchSchedule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/schedule/dani" {
			t.Errorf("path = %q, want /api/v1/schedule/dani", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want bearer key", got)
		}
		if got := r.UserAgent(); got != "timelord-enforcer/test" {
			t.Errorf("User-Agent = %q, want timelord-enforcer/test", got)
		}
		w.Header().Set("X-Schedule-Revision", "42")
		w.Write([]byte(readTestdata(t, "schedule.json")))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test", "test-key")
	schedule, revision, err := client.FetchSchedule(context.Background(), "dani")
	if err != nil {
		t.Fatalf("FetchSchedule() error = %v", err)
	}
	if revision != 42 {
		t.Errorf("revision = %d, want 42", revision)
	}
	if schedule.User != "dani" {
		t.Errorf("User = %q, want dani", schedule.User)
	}
}

func TestClientSendStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status/dani" {
			t.Errorf("path = %q, want /api/v1/status/dani", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want bearer key", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var request StatusRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.EnforcerVersion != "test" || request.Hostname != "test-host" {
			t.Errorf("request = %+v, want test version and hostname", request)
		}

		w.Header().Set("X-Schedule-Revision", "43")
		w.Write([]byte(readTestdata(t, "status", "response.json")))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test", "test-key")
	response, revision, err := client.SendStatus(context.Background(), "dani", StatusRequest{
		ObservedAt:      time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
		EnforcerVersion: "test",
		Hostname:        "test-host",
		UsageSeconds:    0,
		Processes:       []ProcessStat{},
	})
	if err != nil {
		t.Fatalf("SendStatus() error = %v", err)
	}
	if revision != 43 {
		t.Errorf("revision = %d, want 43", revision)
	}
	if response.ServerDay != "2024-06-03" {
		t.Errorf("ServerDay = %q, want 2024-06-03", response.ServerDay)
	}
}

func TestClientErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test", "wrong")
	if _, _, err := client.FetchSchedule(context.Background(), "dani"); err == nil {
		t.Fatal("FetchSchedule() error = nil, want error")
	}
}
