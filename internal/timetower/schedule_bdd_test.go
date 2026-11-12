package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
	"timelord/internal/policy"
)

func registerScheduleSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^the current schedule revision is recorded as "([^"]*)"$`, w.recordRevision)
	sc.Step(`^the schedule revision is still "([^"]*)"$`, w.revisionIsStill)
	sc.Step(`^the schedule revision differs from "([^"]*)"$`, w.revisionDiffers)
	sc.Step(`^the enforcer requests the schedule for "([^"]*)"$`, w.requestSchedule)
	sc.Step(`^the admin requests the schedule with admin credentials for "([^"]*)"$`, w.requestScheduleAsAdmin)
	sc.Step(`^the enforcer requests the schedule for "([^"]*)" (without credentials|with the wrong password)$`, w.requestScheduleWithAuth)
	sc.Step(`^the enforcer sends a POST to the schedule for "([^"]*)"$`, w.postSchedule)
	sc.Step(`^the schedule is:$`, w.scheduleIs)
}

func (w *apiWorld) requestSchedule(user string) error {
	return w.submit(http.MethodGet, "/api/v1/schedule/"+user, nil, authEnforcer)
}

func (w *apiWorld) requestScheduleAsAdmin(user string) error {
	return w.submit(http.MethodGet, "/api/v1/schedule/"+user, nil, authAdmin)
}

func (w *apiWorld) requestScheduleWithAuth(user, credentials string) error {
	authorization := authNone
	if credentials == "with the wrong password" {
		authorization = authEnforcerWrong
	}
	return w.submit(http.MethodGet, "/api/v1/schedule/"+user, nil, authorization)
}

func (w *apiWorld) postSchedule(user string) error {
	return w.submit(http.MethodPost, "/api/v1/schedule/"+user, nil, authEnforcer)
}

func (w *apiWorld) recordRevision(label string) error {
	if err := w.fetchSchedule("dani"); err != nil {
		return err
	}
	w.revisions[label] = w.rec.Header().Get("X-Schedule-Revision")
	return nil
}

func (w *apiWorld) revisionIsStill(label string) error {
	if err := w.fetchSchedule("dani"); err != nil {
		return err
	}
	saved, ok := w.revisions[label]
	if !ok {
		return fmt.Errorf("revision %q was never recorded", label)
	}
	if got := w.rec.Header().Get("X-Schedule-Revision"); got != saved {
		return fmt.Errorf("revision = %q, want unchanged %q", got, saved)
	}
	return nil
}

func (w *apiWorld) revisionDiffers(label string) error {
	if err := w.fetchSchedule("dani"); err != nil {
		return err
	}
	saved, ok := w.revisions[label]
	if !ok {
		return fmt.Errorf("revision %q was never recorded", label)
	}
	if got := w.rec.Header().Get("X-Schedule-Revision"); got == saved {
		return fmt.Errorf("revision = %q, want a different revision", got)
	}
	return nil
}

type scheduleExpectation struct {
	SchemaVersion     *int     `table:"schema_version"`
	Timezone          *string  `table:"timezone"`
	User              *string  `table:"user"`
	Day               *string  `table:"day"`
	UsedSeconds       *int64   `table:"used_seconds"`
	RemainingSeconds  *int64   `table:"remaining_seconds"`
	DailyLimitSeconds *int64   `table:"daily_limit_seconds"`
	Windows           []string `table:"window"`
}

func (w *apiWorld) scheduleIs(tbl *godog.Table) error {
	if err := w.fetchSchedule("dani"); err != nil {
		return err
	}
	var want scheduleExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}

	if want.SchemaVersion != nil && w.schedule.SchemaVersion != *want.SchemaVersion {
		return fmt.Errorf("schema_version = %d, want %d", w.schedule.SchemaVersion, *want.SchemaVersion)
	}
	if want.Timezone != nil && w.schedule.Timezone != *want.Timezone {
		return fmt.Errorf("timezone = %q, want %q", w.schedule.Timezone, *want.Timezone)
	}
	if want.User != nil && w.schedule.User != *want.User {
		return fmt.Errorf("user = %q, want %q", w.schedule.User, *want.User)
	}
	if want.Day != nil && w.schedule.Usage.Day != *want.Day {
		return fmt.Errorf("day = %q, want %q", w.schedule.Usage.Day, *want.Day)
	}
	if want.UsedSeconds != nil && w.schedule.Usage.UsedSeconds != *want.UsedSeconds {
		return fmt.Errorf("used_seconds = %d, want %d", w.schedule.Usage.UsedSeconds, *want.UsedSeconds)
	}
	if want.RemainingSeconds != nil {
		if err := checkSeconds("remaining_seconds", w.schedule.Usage.RemainingSeconds, *want.RemainingSeconds); err != nil {
			return err
		}
	}
	if want.DailyLimitSeconds != nil {
		if w.schedule.Policy == nil {
			return fmt.Errorf("schedule policy is nil")
		}
		if got := w.schedule.Policy.DailyLimitSeconds; got != *want.DailyLimitSeconds {
			return fmt.Errorf("daily_limit_seconds = %d, want %d", got, *want.DailyLimitSeconds)
		}
	}
	if len(want.Windows) == 0 {
		return nil
	}
	if w.schedule.Policy == nil {
		return fmt.Errorf("schedule policy is nil")
	}
	return compareWindows(w.schedule.Policy.Windows, want.Windows)
}

func (w *apiWorld) fetchSchedule(user string) error {
	if err := w.submit(http.MethodGet, "/api/v1/schedule/"+user, nil, authEnforcer); err != nil {
		return err
	}
	if w.rec.Code != http.StatusOK {
		return fmt.Errorf("schedule status = %d, want 200; body=%s", w.rec.Code, w.rec.Body.String())
	}
	w.schedule = Schedule{}
	if err := json.Unmarshal(w.rec.Body.Bytes(), &w.schedule); err != nil {
		return fmt.Errorf("decode schedule: %w", err)
	}
	return nil
}

func compareWindows(got []policy.Window, want []string) error {
	if len(got) != len(want) {
		return fmt.Errorf("windows = %d, want %d", len(got), len(want))
	}
	for i, expected := range want {
		if actual := formatWindow(got[i]); actual != expected {
			return fmt.Errorf("window[%d] = %q, want %q", i, actual, expected)
		}
	}
	return nil
}

func formatWindow(window policy.Window) string {
	return fmt.Sprintf("%s %s-%s", window.Day, window.Start, window.End)
}

func checkSeconds(field string, got *int64, want int64) error {
	if got == nil {
		return fmt.Errorf("%s = nil, want %d", field, want)
	}
	if *got != want {
		return fmt.Errorf("%s = %d, want %d", field, *got, want)
	}
	return nil
}
