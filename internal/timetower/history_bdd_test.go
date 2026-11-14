package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
)

func registerHistorySteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^the admin requests the usage history for "([^"]*)"$`, w.requestHistory)
	sc.Step(`^the admin requests the usage history for "([^"]*)" with days "([^"]*)"$`, w.requestHistoryWithDays)
	sc.Step(`^the usage history is requested without credentials$`, w.requestHistoryWithoutCredentials)
	sc.Step(`^the enforcer requests the usage history with its key$`, w.requestHistoryAsEnforcer)
	sc.Step(`^the admin sends a POST to the usage history$`, w.postHistory)
	sc.Step(`^the usage history is:$`, w.historyIs)
}

func (w *apiWorld) requestHistory(user string) error {
	return w.submit(http.MethodGet, "/api/v1/history/"+user, nil, authAdmin)
}

func (w *apiWorld) requestHistoryWithDays(user, days string) error {
	return w.submit(http.MethodGet, "/api/v1/history/"+user+"?days="+url.QueryEscape(days), nil, authAdmin)
}

func (w *apiWorld) requestHistoryWithoutCredentials() error {
	return w.submit(http.MethodGet, "/api/v1/history/dani", nil, authNone)
}

func (w *apiWorld) requestHistoryAsEnforcer() error {
	return w.submit(http.MethodGet, "/api/v1/history/dani", nil, authEnforcer)
}

func (w *apiWorld) postHistory() error {
	return w.submit(http.MethodPost, "/api/v1/history/dani", nil, authAdmin)
}

type historyExpectation struct {
	User    *string  `table:"user"`
	Entries []string `table:"entry"`
}

func (w *apiWorld) historyIs(tbl *godog.Table) error {
	if w.rec == nil {
		return fmt.Errorf("no history response")
	}
	if w.rec.Code != http.StatusOK {
		return fmt.Errorf("history status = %d, want 200; body=%s", w.rec.Code, w.rec.Body.String())
	}

	var want historyExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}

	var history UsageHistory
	if err := json.Unmarshal(w.rec.Body.Bytes(), &history); err != nil {
		return fmt.Errorf("decode usage history: %w", err)
	}
	if want.User != nil && history.User != *want.User {
		return fmt.Errorf("user = %q, want %q", history.User, *want.User)
	}

	if len(history.History) != len(want.Entries) {
		return fmt.Errorf("history entries = %d, want %d", len(history.History), len(want.Entries))
	}
	for i, raw := range want.Entries {
		parts := strings.Fields(raw)
		if len(parts) != 2 {
			return fmt.Errorf("entry %q must be \"<day> <seconds>\"", raw)
		}
		used, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return fmt.Errorf("entry %q: %w", raw, err)
		}
		got := history.History[i]
		if got.Day != parts[0] || got.UsedSeconds != used {
			return fmt.Errorf("history[%d] = %s %d, want %s %d", i, got.Day, got.UsedSeconds, parts[0], used)
		}
	}
	return nil
}
