package timetower

import "time"

// Schedule is the GET /api/v1/schedule response body.
type Schedule struct {
	SchemaVersion         int               `json:"schema_version"`
	ServerTime            time.Time         `json:"server_time"`
	Timezone              string            `json:"timezone"`
	StatusIntervalSeconds int               `json:"status_interval_seconds"`
	OfflineGraceSeconds   int               `json:"offline_grace_seconds"`
	Subjects              []ScheduleSubject `json:"subjects"`
}

// ScheduleSubject is one subject in a schedule response.
type ScheduleSubject struct {
	Subject string     `json:"subject"`
	Account string     `json:"account"`
	Policy  *Policy    `json:"policy"`
	Usage   UsageState `json:"usage"`
}

// UsageState is a subject's usage for a policy day.
type UsageState struct {
	Day              string `json:"day"`
	UsedSeconds      int64  `json:"used_seconds"`
	RemainingSeconds *int64 `json:"remaining_seconds"`
}

// StatusRequest is the PUT /api/v1/status request body.
type StatusRequest struct {
	ObservedAt      time.Time       `json:"observed_at"`
	EnforcerVersion string          `json:"enforcer_version"`
	Health          Health          `json:"health"`
	Subjects        []StatusSubject `json:"subjects"`
	CollectedInMS   int64           `json:"collected_in_ms,omitempty"`
	Processes       []ProcessStat   `json:"processes"`
}

// Health is the Enforcer's reported health.
type Health struct {
	Status    string  `json:"status"`
	LastError *string `json:"last_error"`
}

// StatusSubject is one subject's reported state.
type StatusSubject struct {
	Subject        string       `json:"subject"`
	Account        string       `json:"account"`
	AccountPresent bool         `json:"account_present"`
	SessionCount   int          `json:"session_count"`
	Active         bool         `json:"active"`
	Locked         bool         `json:"locked"`
	Usage          []UsageEntry `json:"usage"`
}

// UsageEntry is an absolute usage counter for one policy day.
type UsageEntry struct {
	Day         string `json:"day"`
	UsedSeconds int64  `json:"used_seconds"`
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

// StatusResponse is the PUT /api/v1/status response body.
type StatusResponse struct {
	ServerTime time.Time               `json:"server_time"`
	ServerDay  string                  `json:"server_day"`
	Subjects   []StatusResponseSubject `json:"subjects"`
}

// StatusResponseSubject is the server's authoritative state for one subject.
type StatusResponseSubject struct {
	Subject              string   `json:"subject"`
	CurrentDay           string   `json:"current_day"`
	ReportedUsedSeconds  int64    `json:"reported_used_seconds"`
	AggregateUsedSeconds int64    `json:"aggregate_used_seconds"`
	DailyLimitSeconds    int64    `json:"daily_limit_seconds"`
	RemainingSeconds     *int64   `json:"remaining_seconds"`
	InsideWindow         bool     `json:"inside_window"`
	Allowed              bool     `json:"allowed"`
	DenyReason           *string  `json:"deny_reason"`
	AcknowledgedDays     []string `json:"acknowledged_days"`
}

// EnforcerList is the GET /api/v1/enforcers response body.
type EnforcerList struct {
	ServerTime time.Time        `json:"server_time"`
	ServerDay  string           `json:"server_day"`
	Enforcers  []EnforcerStatus `json:"enforcers"`
}

// EnforcerStatus is the current state of one configured Enforcer.
type EnforcerStatus struct {
	IP       string                  `json:"ip"`
	Name     string                  `json:"name"`
	Online   bool                    `json:"online"`
	LastSeen *time.Time              `json:"last_seen"`
	Subjects []EnforcerSubjectStatus `json:"subjects"`
}

// EnforcerSubjectStatus is the current schedule state for one subject.
type EnforcerSubjectStatus struct {
	Subject           string  `json:"subject"`
	Account           string  `json:"account"`
	UsedSeconds       int64   `json:"used_seconds"`
	DailyLimitSeconds int64   `json:"daily_limit_seconds"`
	RemainingSeconds  *int64  `json:"remaining_seconds"`
	InsideWindow      bool    `json:"inside_window"`
	Allowed           bool    `json:"allowed"`
	DenyReason        *string `json:"deny_reason"`
}
