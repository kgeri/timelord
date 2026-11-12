package timetower

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

const (
	testEnforcerKey   = "test-enforcer-key"
	testAdminPassword = "test-admin-password"
)

const (
	authNone          = ""
	authEnforcer      = "enforcer"
	authEnforcerWrong = "enforcer-wrong"
	authAdmin         = "admin"
	authAdminWrong    = "admin-wrong"
)

// TestAPIFeatures runs the Timetower HTTP behavior specs from testdata/features.
func TestAPIFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "timetower api",
		ScenarioInitializer: initializeAPIScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{testdataPath("features")},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("api features failed")
	}
}

type apiWorld struct {
	server    *Server
	config    *Config
	store     *Store
	now       time.Time
	stateDir  string
	statePath string
	rec       *httptest.ResponseRecorder
	statusRec *httptest.ResponseRecorder
	schedule  Schedule
	status    StatusResponse
	listing   EnforcerList
	users     UserList
	revisions map[string]string
}

func initializeAPIScenario(sc *godog.ScenarioContext) {
	w := &apiWorld{}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*w = apiWorld{}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.stateDir != "" {
			os.RemoveAll(w.stateDir)
		}
		return ctx, nil
	})

	registerServerSteps(sc, w)
	registerScheduleSteps(sc, w)
	registerStatusSteps(sc, w)
	registerListingSteps(sc, w)
	registerUsersSteps(sc, w)
	registerOverrideSteps(sc, w)
}

func registerServerSteps(sc *godog.ScenarioContext, w *apiWorld) {
	sc.Step(`^a Timetower server with configuration "([^"]*)" at "([^"]*)"$`, w.startServer)
	sc.Step(`^a Timetower server with state and configuration "([^"]*)" at "([^"]*)"$`, w.startServerWithState)
	sc.Step(`^the server is restarted$`, w.restartServer)
	sc.Step(`^the server clock advances to "([^"]*)"$`, w.advanceClock)
	sc.Step(`^the server checks for heartbeat timeouts after "([^"]*)"$`, w.checkHeartbeatTimeouts)
	sc.Step(`^the response status is (\d+)$`, w.responseStatusIs)
	sc.Step(`^the response header "([^"]*)" is not empty$`, w.responseHeaderNotEmpty)
	sc.Step(`^the Allow header is "([^"]*)"$`, w.allowHeaderIs)
	sc.Step(`^the stored usage for "([^"]*)" on "([^"]*)" is (\d+) seconds$`, w.storedUsage)
	sc.Step(`^the stored override for "([^"]*)" on "([^"]*)" has limit (\d+) minutes$`, w.storedOverride)
}

func (w *apiWorld) startServer(configFile, timestamp string) error {
	return w.startServerWithStore(configFile, timestamp, "")
}

func (w *apiWorld) startServerWithState(configFile, timestamp string) error {
	dir, err := os.MkdirTemp("", "timetower-bdd-")
	if err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	w.stateDir = dir
	w.statePath = filepath.Join(dir, "state.csv")
	return w.startServerWithStore(configFile, timestamp, w.statePath)
}

func (w *apiWorld) startServerWithStore(configFile, timestamp, statePath string) error {
	now, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", timestamp, err)
	}
	config, err := LoadConfig(testdataPath(configFile))
	if err != nil {
		return fmt.Errorf("load config %q: %w", configFile, err)
	}
	store, err := NewStore(statePath)
	if err != nil {
		return fmt.Errorf("new store: %w", err)
	}
	server := NewServer(config, store, testEnforcerKey, testAdminPassword)
	server.now = func() time.Time { return w.now }
	w.server, w.config, w.store, w.now = server, config, store, now
	w.revisions = make(map[string]string)
	return nil
}

func (w *apiWorld) restartServer() error {
	if w.statePath == "" {
		return fmt.Errorf("server has no state file")
	}
	store, err := NewStore(w.statePath)
	if err != nil {
		return fmt.Errorf("reload store: %w", err)
	}
	server := NewServer(w.config, store, testEnforcerKey, testAdminPassword)
	server.now = func() time.Time { return w.now }
	w.server, w.store = server, store
	return nil
}

func (w *apiWorld) advanceClock(timestamp string) error {
	now, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("parse timestamp %q: %w", timestamp, err)
	}
	w.now = now
	return nil
}

func (w *apiWorld) checkHeartbeatTimeouts(duration string) error {
	d, err := time.ParseDuration(duration)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", duration, err)
	}
	w.server.checkTimeouts(w.now.Add(d))
	return nil
}

func (w *apiWorld) responseStatusIs(want int) error {
	if w.rec == nil {
		return fmt.Errorf("no response")
	}
	if w.rec.Code != want {
		return fmt.Errorf("status = %d, want %d; body=%s", w.rec.Code, want, w.rec.Body.String())
	}
	return nil
}

func (w *apiWorld) responseHeaderNotEmpty(header string) error {
	if w.rec == nil {
		return fmt.Errorf("no response")
	}
	if got := w.rec.Header().Get(header); got == "" {
		return fmt.Errorf("header %s is empty", header)
	}
	return nil
}

func (w *apiWorld) allowHeaderIs(want string) error {
	if w.rec == nil {
		return fmt.Errorf("no response")
	}
	if got := w.rec.Header().Get("Allow"); got != want {
		return fmt.Errorf("Allow = %q, want %q", got, want)
	}
	return nil
}

func (w *apiWorld) storedUsage(user, day string, want int64) error {
	if got := w.store.Usage("test-host", user, day); got != want {
		return fmt.Errorf("stored usage = %d, want %d", got, want)
	}
	return nil
}

func (w *apiWorld) storedOverride(user, day string, want int64) error {
	override, ok := w.store.Override(user, day)
	if !ok {
		return fmt.Errorf("override for %s on %s is unset", user, day)
	}
	if override.LimitMinutes == nil || *override.LimitMinutes != want {
		return fmt.Errorf("override limit = %v, want %d minutes", override.LimitMinutes, want)
	}
	return nil
}

func (w *apiWorld) submit(method, path string, body []byte, authorization string) error {
	if w.server == nil {
		return fmt.Errorf("no server")
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	switch authorization {
	case authEnforcer:
		req.SetBasicAuth(enforcerUsername, testEnforcerKey)
	case authEnforcerWrong:
		req.SetBasicAuth(enforcerUsername, "wrong")
	case authAdmin:
		req.SetBasicAuth(adminUsername, testAdminPassword)
	case authAdminWrong:
		req.SetBasicAuth(adminUsername, "wrong")
	}
	rec := httptest.NewRecorder()
	w.server.ServeHTTP(rec, req)
	w.rec = rec
	return nil
}
