package timetower

import (
	"time"

	"timelord/internal/policy"
)

// Schedule is the GET /api/v1/schedule/{user} response body.
type Schedule struct {
	SchemaVersion         int        `json:"schema_version"`
	ServerTime            time.Time  `json:"server_time"`
	Timezone              string     `json:"timezone"`
	StatusIntervalSeconds int        `json:"status_interval_seconds"`
	OfflineGraceSeconds   int        `json:"offline_grace_seconds"`
	User                  string     `json:"user"`
	Policy                *Policy    `json:"policy"`
	Usage                 UsageState `json:"usage"`
}

// Policy is a user's policy as sent to Enforcers. DailyLimitSeconds is the
// limit for the schedule's usage day, converted from the configured
// per-weekday minutes.
type Policy struct {
	Enabled           bool            `json:"enabled"`
	DailyLimitSeconds int64           `json:"daily_limit_seconds"`
	Windows           []policy.Window `json:"windows"`
}

// UsageState is a user's usage for a policy day.
type UsageState struct {
	Day              string `json:"day"`
	UsedSeconds      int64  `json:"used_seconds"`
	RemainingSeconds *int64 `json:"remaining_seconds"`
}

// StatusRequest is the PUT /api/v1/status/{user} request body.
type StatusRequest struct {
	ObservedAt      time.Time     `json:"observed_at"`
	EnforcerVersion string        `json:"enforcer_version"`
	Hostname        string        `json:"hostname"`
	Active          bool          `json:"active"`
	Locked          bool          `json:"locked"`
	SessionCount    int           `json:"session_count"`
	UsageSeconds    int64         `json:"usage_seconds"`
	CollectedInMS   int64         `json:"collected_in_ms,omitempty"`
	Processes       []ProcessStat `json:"processes"`
}

// ProcessStat is one aggregate process series.
type ProcessStat struct {
	User            string  `json:"user"`
	Name            string  `json:"name"`
	Scope           string  `json:"scope"`
	Instances       int     `json:"instances"`
	MemoryRSSBytes  int64   `json:"memory_rss_bytes"`
	CPUSecondsTotal float64 `json:"cpu_seconds_total"`
}

// StatusResponse is the PUT /api/v1/status/{user} response body.
type StatusResponse struct {
	ServerTime           time.Time `json:"server_time"`
	ServerDay            string    `json:"server_day"`
	User                 string    `json:"user"`
	CurrentDay           string    `json:"current_day"`
	ReportedUsedSeconds  int64     `json:"reported_used_seconds"`
	AggregateUsedSeconds int64     `json:"aggregate_used_seconds"`
	DailyLimitSeconds    int64     `json:"daily_limit_seconds"`
	RemainingSeconds     *int64    `json:"remaining_seconds"`
	InsideWindow         bool      `json:"inside_window"`
	Allowed              bool      `json:"allowed"`
	DenyReason           *string   `json:"deny_reason"`
}

// EnforcerList is the public GET /api/v1/enforcers response body.
type EnforcerList struct {
	ServerTime time.Time        `json:"server_time"`
	ServerDay  string           `json:"server_day"`
	Enforcers  []EnforcerStatus `json:"enforcers"`
}

// EnforcerStatus is the current state of one reporting hostname.
type EnforcerStatus struct {
	Hostname string       `json:"hostname"`
	Online   bool         `json:"online"`
	LastSeen *time.Time   `json:"last_seen"`
	Users    []UserStatus `json:"users"`
}

// UserStatus is the current schedule state for one user.
type UserStatus struct {
	User              string  `json:"user"`
	UsedSeconds       int64   `json:"used_seconds"`
	DailyLimitSeconds int64   `json:"daily_limit_seconds"`
	RemainingSeconds  *int64  `json:"remaining_seconds"`
	InsideWindow      bool    `json:"inside_window"`
	Allowed           bool    `json:"allowed"`
	DenyReason        *string `json:"deny_reason"`
}
