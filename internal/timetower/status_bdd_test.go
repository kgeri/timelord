package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
)

func registerStatusSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^the enforcer reports:$`, w.enforcerReports)
	sc.Step(`^the enforcer submits report file "([^"]*)" for user "([^"]*)"$`, w.enforcerSubmitsReportFile)
	sc.Step(`^the status is:$`, w.statusIs)
	sc.Step(`^the status has a revision$`, w.statusHasRevision)
}

type reportInput struct {
	User     *string `table:"user"`
	Hostname *string `table:"hostname"`
	Day      *string `table:"day"`
	Used     *int64  `table:"used"`
}

func (w *apiWorld) enforcerReports(tbl *godog.Table) error {
	var input reportInput
	if err := gherkin.DecodeTable(tbl, &input); err != nil {
		return err
	}
	if input.Day == nil || input.Used == nil {
		return fmt.Errorf("report needs day and used")
	}
	user := "dani"
	if input.User != nil {
		user = *input.User
	}

	request, err := statusRequest(*input.Used, *input.Day)
	if err != nil {
		return err
	}
	if input.Hostname != nil {
		request.Hostname = *input.Hostname
	}
	body, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Errorf("encode status request: %w", err)
	}

	w.status = StatusResponse{}
	w.statusRec = nil
	if err := w.submit(http.MethodPut, "/api/v1/status/"+user, body, authBearer); err != nil {
		return err
	}
	w.statusRec = w.rec
	if w.rec.Code == http.StatusOK {
		if err := json.Unmarshal(w.rec.Body.Bytes(), &w.status); err != nil {
			return fmt.Errorf("decode status: %w", err)
		}
	}
	return nil
}

func (w *apiWorld) enforcerSubmitsReportFile(fixture, user string) error {
	body, err := os.ReadFile(testdataPath("status", fixture))
	if err != nil {
		return fmt.Errorf("read status fixture %q: %w", fixture, err)
	}
	w.statusRec = nil
	return w.submit(http.MethodPut, "/api/v1/status/"+user, body, authBearer)
}

type statusExpectation struct {
	User                 *string `table:"user"`
	Allowed              *bool   `table:"allowed"`
	InsideWindow         *bool   `table:"inside_window"`
	DenyReason           *string `table:"deny_reason"`
	DailyLimitSeconds    *int64  `table:"daily_limit_seconds"`
	RemainingSeconds     *int64  `table:"remaining_seconds"`
	ReportedUsedSeconds  *int64  `table:"reported_used_seconds"`
	AggregateUsedSeconds *int64  `table:"aggregate_used_seconds"`
}

func (w *apiWorld) statusIs(tbl *godog.Table) error {
	if w.statusRec == nil || w.statusRec.Code != http.StatusOK {
		code, body := 0, ""
		if w.statusRec != nil {
			code, body = w.statusRec.Code, w.statusRec.Body.String()
		}
		return fmt.Errorf("report status = %d, want 200; body=%s", code, body)
	}
	var want statusExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}

	if want.User != nil && w.status.User != *want.User {
		return fmt.Errorf("user = %q, want %q", w.status.User, *want.User)
	}
	if want.Allowed != nil && w.status.Allowed != *want.Allowed {
		return fmt.Errorf("allowed = %v, want %v", w.status.Allowed, *want.Allowed)
	}
	if want.InsideWindow != nil && w.status.InsideWindow != *want.InsideWindow {
		return fmt.Errorf("inside_window = %v, want %v", w.status.InsideWindow, *want.InsideWindow)
	}
	if want.DenyReason != nil {
		if got := stringValue(w.status.DenyReason); got != *want.DenyReason {
			return fmt.Errorf("deny_reason = %q, want %q", got, *want.DenyReason)
		}
	}
	if want.DailyLimitSeconds != nil && w.status.DailyLimitSeconds != *want.DailyLimitSeconds {
		return fmt.Errorf("daily_limit_seconds = %d, want %d", w.status.DailyLimitSeconds, *want.DailyLimitSeconds)
	}
	if want.RemainingSeconds != nil {
		if err := checkSeconds("remaining_seconds", w.status.RemainingSeconds, *want.RemainingSeconds); err != nil {
			return err
		}
	}
	if want.ReportedUsedSeconds != nil && w.status.ReportedUsedSeconds != *want.ReportedUsedSeconds {
		return fmt.Errorf("reported_used_seconds = %d, want %d", w.status.ReportedUsedSeconds, *want.ReportedUsedSeconds)
	}
	if want.AggregateUsedSeconds != nil && w.status.AggregateUsedSeconds != *want.AggregateUsedSeconds {
		return fmt.Errorf("aggregate_used_seconds = %d, want %d", w.status.AggregateUsedSeconds, *want.AggregateUsedSeconds)
	}
	return nil
}

func (w *apiWorld) statusHasRevision() error {
	if w.statusRec == nil || w.statusRec.Code != http.StatusOK {
		return fmt.Errorf("no successful status response")
	}
	if got := w.statusRec.Header().Get("X-Schedule-Revision"); got == "" {
		return fmt.Errorf("X-Schedule-Revision is empty")
	}
	return nil
}

func statusRequest(used int64, day string) (StatusRequest, error) {
	data, err := os.ReadFile(testdataPath("status", "request.json"))
	if err != nil {
		return StatusRequest{}, fmt.Errorf("read status request: %w", err)
	}
	var request StatusRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return StatusRequest{}, fmt.Errorf("decode status request: %w", err)
	}
	observed, err := time.Parse(dateLayout, day)
	if err != nil {
		return StatusRequest{}, fmt.Errorf("parse day %q: %w", day, err)
	}
	request.ObservedAt = observed.Add(10 * time.Hour)
	request.UsageSeconds = used
	return request, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
