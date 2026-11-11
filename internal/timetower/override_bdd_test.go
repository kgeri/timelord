package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
	"timelord/internal/policy"
)

func registerOverrideSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^the admin sets the override:$`, w.adminSetsOverride)
	sc.Step(`^the admin submits an unauthenticated override:$`, w.adminSubmitsUnauthenticatedOverride)
	sc.Step(`^the admin submits a (PUT|POST) override file "([^"]*)" for user "([^"]*)"$`, w.adminSubmitsOverrideFile)
	sc.Step(`^the override is:$`, w.overrideIs)
}

type overrideInput struct {
	Start        *string `table:"start"`
	End          *string `table:"end"`
	LimitMinutes *int64  `table:"limit_minutes"`
}

type overrideExpectation struct {
	User         *string  `table:"user"`
	Day          *string  `table:"day"`
	LimitMinutes *int64   `table:"limit_minutes"`
	Windows      []string `table:"window"`
}

func (w *apiWorld) adminSetsOverride(tbl *godog.Table) error {
	body, err := overrideBody(tbl)
	if err != nil {
		return err
	}
	return w.submit(http.MethodPut, "/api/v1/override/dani", body, authBearer)
}

func (w *apiWorld) adminSubmitsUnauthenticatedOverride(tbl *godog.Table) error {
	body, err := overrideBody(tbl)
	if err != nil {
		return err
	}
	return w.submit(http.MethodPut, "/api/v1/override/dani", body, authNone)
}

func (w *apiWorld) adminSubmitsOverrideFile(method, fixture, user string) error {
	body, err := os.ReadFile(testdataPath("override", fixture))
	if err != nil {
		return fmt.Errorf("read override fixture %q: %w", fixture, err)
	}
	return w.submit(method, "/api/v1/override/"+user, body, authBearer)
}

func overrideBody(tbl *godog.Table) ([]byte, error) {
	var input overrideInput
	if err := gherkin.DecodeTable(tbl, &input); err != nil {
		return nil, err
	}

	var request OverrideRequest
	if input.Start != nil {
		clock, err := policy.ParseClock(*input.Start)
		if err != nil {
			return nil, fmt.Errorf("start %q: %w", *input.Start, err)
		}
		request.Start = &clock
	}
	if input.End != nil {
		clock, err := policy.ParseClock(*input.End)
		if err != nil {
			return nil, fmt.Errorf("end %q: %w", *input.End, err)
		}
		request.End = &clock
	}
	request.LimitMinutes = input.LimitMinutes

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode override request: %w", err)
	}
	return body, nil
}

func (w *apiWorld) overrideIs(tbl *godog.Table) error {
	if w.rec == nil {
		return fmt.Errorf("no response")
	}
	if w.rec.Code != http.StatusOK {
		return fmt.Errorf("status = %d, want 200; body=%s", w.rec.Code, w.rec.Body.String())
	}
	var got Override
	if err := json.Unmarshal(w.rec.Body.Bytes(), &got); err != nil {
		return fmt.Errorf("decode override response: %w", err)
	}
	var want overrideExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}

	if want.User != nil && got.User != *want.User {
		return fmt.Errorf("user = %q, want %q", got.User, *want.User)
	}
	if want.Day != nil && got.Day != *want.Day {
		return fmt.Errorf("day = %q, want %q", got.Day, *want.Day)
	}
	if want.LimitMinutes != nil && got.LimitMinutes != *want.LimitMinutes {
		return fmt.Errorf("limit_minutes = %d, want %d", got.LimitMinutes, *want.LimitMinutes)
	}
	if len(want.Windows) == 0 {
		return nil
	}
	return compareWindows(got.Windows, want.Windows)
}
