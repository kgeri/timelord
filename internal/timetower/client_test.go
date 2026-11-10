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
		if r.URL.Path != "/api/v1/schedule" {
			t.Errorf("path = %q, want /api/v1/schedule", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got := r.UserAgent(); got != "timelord-enforcer/test" {
			t.Errorf("User-Agent = %q, want timelord-enforcer/test", got)
		}
		w.Header().Set("X-Schedule-Revision", "42")
		json.NewEncoder(w).Encode(Schedule{
			SchemaVersion: 1,
			ServerTime:    time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
			Timezone:      "UTC",
			Subjects:      []ScheduleSubject{},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test")
	schedule, revision, err := client.FetchSchedule(context.Background())
	if err != nil {
		t.Fatalf("FetchSchedule() error = %v", err)
	}
	if revision != 42 {
		t.Errorf("revision = %d, want 42", revision)
	}
	if schedule.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC", schedule.Timezone)
	}
}

func TestClientSendStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("path = %q, want /api/v1/status", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}

		var request StatusRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.EnforcerVersion != "test" {
			t.Errorf("EnforcerVersion = %q, want test", request.EnforcerVersion)
		}

		w.Header().Set("X-Schedule-Revision", "43")
		json.NewEncoder(w).Encode(StatusResponse{
			ServerTime: time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
			ServerDay:  "2024-06-03",
			Subjects:   []StatusResponseSubject{},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test")
	response, revision, err := client.SendStatus(context.Background(), StatusRequest{
		ObservedAt:      time.Date(2024, time.June, 3, 10, 0, 0, 0, time.UTC),
		EnforcerVersion: "test",
		Health:          Health{Status: "ok"},
		Subjects:        []StatusSubject{},
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
		http.Error(w, "unknown enforcer", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test")
	if _, _, err := client.FetchSchedule(context.Background()); err == nil {
		t.Fatal("FetchSchedule() error = nil, want error")
	}
}
