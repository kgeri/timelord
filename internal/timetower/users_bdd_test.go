package timetower

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"

	"timelord/internal/gherkin"
)

func registerUsersSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^the admin requests the user listing$`, w.requestUserListing)
	sc.Step(`^the user listing is requested without credentials$`, w.requestUserListingWithoutCredentials)
	sc.Step(`^the enforcer requests the user listing with its key$`, w.requestUserListingAsEnforcer)
	sc.Step(`^the admin sends a POST to the user listing$`, w.postUserListing)
	sc.Step(`^the user listing is:$`, w.userListingIs)
}

func (w *apiWorld) requestUserListing() error {
	return w.submit(http.MethodGet, "/api/v1/users", nil, authAdmin)
}

func (w *apiWorld) requestUserListingWithoutCredentials() error {
	return w.submit(http.MethodGet, "/api/v1/users", nil, authNone)
}

func (w *apiWorld) requestUserListingAsEnforcer() error {
	return w.submit(http.MethodGet, "/api/v1/users", nil, authEnforcer)
}

func (w *apiWorld) postUserListing() error {
	return w.submit(http.MethodPost, "/api/v1/users", nil, authAdmin)
}

type userExpectation struct {
	User        *string `table:"user"`
	DisplayName *string `table:"display_name"`
}

func (w *apiWorld) userListingIs(tbl *godog.Table) error {
	if err := w.fetchUserListing(); err != nil {
		return err
	}
	var want userExpectation
	if err := gherkin.DecodeTable(tbl, &want); err != nil {
		return err
	}
	if len(w.users.Users) != 1 {
		return fmt.Errorf("users = %d, want 1", len(w.users.Users))
	}

	got := w.users.Users[0]
	if want.User != nil && got.User != *want.User {
		return fmt.Errorf("user = %q, want %q", got.User, *want.User)
	}
	if want.DisplayName != nil && got.DisplayName != *want.DisplayName {
		return fmt.Errorf("display_name = %q, want %q", got.DisplayName, *want.DisplayName)
	}
	return nil
}

func (w *apiWorld) fetchUserListing() error {
	if err := w.submit(http.MethodGet, "/api/v1/users", nil, authAdmin); err != nil {
		return err
	}
	if w.rec.Code != http.StatusOK {
		return fmt.Errorf("user listing status = %d, want 200; body=%s", w.rec.Code, w.rec.Body.String())
	}
	w.users = UserList{}
	if err := json.Unmarshal(w.rec.Body.Bytes(), &w.users); err != nil {
		return fmt.Errorf("decode user listing: %w", err)
	}
	return nil
}
