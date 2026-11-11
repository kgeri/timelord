package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
)

func registerListingSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^anyone requests the enforcer listing$`, w.requestListing)
	sc.Step(`^the listing has no enforcers$`, w.listingIsEmpty)
	sc.Step(`^the listing is:$`, w.listingIs)
}

func (w *apiWorld) requestListing() error {
	return w.submit(http.MethodGet, "/api/v1/enforcers", nil, authNone)
}

func (w *apiWorld) listingIsEmpty() error {
	if err := w.fetchListing(); err != nil {
		return err
	}
	if got := len(w.listing.Enforcers); got != 0 {
		return fmt.Errorf("enforcers = %d, want 0", got)
	}
	return nil
}

type listingExpectation struct {
	Hostname          *string `table:"hostname"`
	Online            *bool   `table:"online"`
	LastSeen          *bool   `table:"last_seen"`
	User              *string `table:"user"`
	UsedSeconds       *int64  `table:"used_seconds"`
	RemainingSeconds  *int64  `table:"remaining_seconds"`
	DailyLimitSeconds *int64  `table:"daily_limit_seconds"`
}

func (w *apiWorld) listingIs(tbl *godog.Table) error {
	if err := w.fetchListing(); err != nil {
		return err
	}
	var want listingExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}

	enforcer, err := findEnforcer(w.listing, want.Hostname)
	if err != nil {
		return err
	}
	if want.Online != nil && enforcer.Online != *want.Online {
		return fmt.Errorf("online = %v, want %v", enforcer.Online, *want.Online)
	}
	if want.LastSeen != nil && (enforcer.LastSeen != nil) != *want.LastSeen {
		return fmt.Errorf("last_seen present = %v, want %v", enforcer.LastSeen != nil, *want.LastSeen)
	}

	user, err := findUser(enforcer, want.User)
	if err != nil {
		return err
	}
	if want.UsedSeconds != nil && user.UsedSeconds != *want.UsedSeconds {
		return fmt.Errorf("used_seconds = %d, want %d", user.UsedSeconds, *want.UsedSeconds)
	}
	if want.RemainingSeconds != nil {
		if err := checkSeconds("remaining_seconds", user.RemainingSeconds, *want.RemainingSeconds); err != nil {
			return err
		}
	}
	if want.DailyLimitSeconds != nil && user.DailyLimitSeconds != *want.DailyLimitSeconds {
		return fmt.Errorf("daily_limit_seconds = %d, want %d", user.DailyLimitSeconds, *want.DailyLimitSeconds)
	}
	return nil
}

func (w *apiWorld) fetchListing() error {
	if err := w.submit(http.MethodGet, "/api/v1/enforcers", nil, authNone); err != nil {
		return err
	}
	if w.rec.Code != http.StatusOK {
		return fmt.Errorf("listing status = %d, want 200; body=%s", w.rec.Code, w.rec.Body.String())
	}
	w.listing = EnforcerList{}
	if err := json.Unmarshal(w.rec.Body.Bytes(), &w.listing); err != nil {
		return fmt.Errorf("decode listing: %w", err)
	}
	return nil
}

func findEnforcer(list EnforcerList, hostname *string) (*EnforcerStatus, error) {
	if hostname != nil {
		for i := range list.Enforcers {
			if list.Enforcers[i].Hostname == *hostname {
				return &list.Enforcers[i], nil
			}
		}
		return nil, fmt.Errorf("enforcer %q not found", *hostname)
	}
	if len(list.Enforcers) != 1 {
		return nil, fmt.Errorf("enforcers = %d, want 1", len(list.Enforcers))
	}
	return &list.Enforcers[0], nil
}

func findUser(enforcer *EnforcerStatus, user *string) (*UserStatus, error) {
	if user != nil {
		for i := range enforcer.Users {
			if enforcer.Users[i].User == *user {
				return &enforcer.Users[i], nil
			}
		}
		return nil, fmt.Errorf("user %q not found", *user)
	}
	if len(enforcer.Users) != 1 {
		return nil, fmt.Errorf("users = %d, want 1", len(enforcer.Users))
	}
	return &enforcer.Users[0], nil
}
