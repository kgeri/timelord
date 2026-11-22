package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // embed IANA time zone data for Windows deployments

	"timelord/internal/enforcer"
	"timelord/internal/metrics"
	"timelord/internal/notify"
	"timelord/internal/process"
	"timelord/internal/scheduler"
	"timelord/internal/selftest"
	"timelord/internal/session"
	"timelord/internal/timetower"
)

const enforcerVersion = "0.2.0"

func main() {
	listen := flag.String("listen", "0.0.0.0:9220", "address for the metrics endpoint")
	timetowerURL := flag.String("timetower", "https://timetower.ogreg.org", "Timetower base URL")
	keyFile := flag.String("key-file", "", "read the Enforcer key from this file (default: TIMELORD_ENFORCER_KEY_FILE)")
	selfTestOnly := flag.Bool("selftest", false, "run the startup self-test and exit")
	notifyChild := flag.Bool("notify", false, "internal: show a notification in the current session and exit")
	notifyKind := flag.String("notify-kind", string(notify.KindLimit), "internal: limit or window")
	notifyMinutes := flag.Int("notify-minutes", 5, "internal: minutes remaining for the notice")
	notifyTest := flag.Bool("notify-test", false, "send a test notification to active local sessions and exit")
	var users userList
	flag.Var(&users, "user", "account to manage; use local=server to map accounts; repeatable")
	flag.Parse()

	if *notifyChild {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := notify.RunChild(ctx, *notifyKind, *notifyMinutes); err != nil {
			log.Printf("notify: %v", err)
			os.Exit(1)
		}
		return
	}

	if *notifyTest {
		if err := runNotifyTest(*notifyKind, *notifyMinutes); err != nil {
			log.Fatalf("notify test: %v", err)
		}
		return
	}

	if *selfTestOnly {
		runSelfTest(*listen)
		return
	}

	runPlatform(func(ctx context.Context) error {
		log.Println("TimeLord starting...")

		cache := process.NewCache()
		lister := process.NewLister()
		metricsServer := metrics.NewServer(*listen, cache)

		if err := selftest.Run(selftest.Config{
			Privileged: isPrivileged(),
			List:       lister.List,
			Listen:     metricsServer.Listen,
		}); err != nil {
			return fmt.Errorf("self-test failed: %w", err)
		}

		go metricsServer.Run(ctx)

		sched := scheduler.New(lister, cache)
		sched.Update()
		go sched.Run(ctx)

		sessionSource, err := session.NewSessionSource()
		if err != nil {
			log.Printf("WARN: Timetower enforcement disabled: %v", err)
			<-ctx.Done()
			log.Println("TimeLord stopped")
			return nil
		}

		if len(users) == 0 {
			return fmt.Errorf("no -user option given")
		}
		enforcerKey, err := loadEnforcerKey(*keyFile)
		if err != nil {
			return err
		}
		hostname := strings.TrimSpace(os.Getenv("TIMELORD_HOSTNAME"))
		if hostname == "" {
			var err error
			hostname, err = os.Hostname()
			if err != nil {
				return fmt.Errorf("get hostname: %w", err)
			}
		}

		agent := enforcer.New(enforcer.Config{
			Client:    timetower.NewClient(*timetowerURL, enforcerVersion, enforcerKey),
			Sessions:  sessionSource,
			Processes: cache,
			Notifier:  notify.New(),
			Version:   enforcerVersion,
			Hostname:  hostname,
			Users:     users,
		})
		if err := agent.Run(ctx); err != nil {
			return err
		}

		log.Println("TimeLord stopped")
		return nil
	})
}

// loadEnforcerKey returns the Enforcer key from an explicit file, the
// TIMELORD_ENFORCER_KEY_FILE file, or the TIMELORD_ENFORCER_KEY environment
// variable, in that order.
func loadEnforcerKey(path string) (string, error) {
	if path == "" {
		path = strings.TrimSpace(os.Getenv("TIMELORD_ENFORCER_KEY_FILE"))
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read Enforcer key file: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("Enforcer key file %s is empty", path)
		}
		return key, nil
	}

	key := strings.TrimSpace(os.Getenv("TIMELORD_ENFORCER_KEY"))
	if key == "" {
		return "", fmt.Errorf("TIMELORD_ENFORCER_KEY is not set and no key file was given")
	}
	return key, nil
}

// userList collects a repeatable -user flag.
type userList []enforcer.User

func (u *userList) String() string {
	values := make([]string, len(*u))
	for i, user := range *u {
		values[i] = user.String()
	}
	return strings.Join(values, ",")
}

func (u *userList) Set(value string) error {
	user, err := enforcer.ParseUser(value)
	if err != nil {
		return err
	}
	*u = append(*u, user)
	return nil
}

// runNotifyTest sends one notice to every active local session. It is a
// debugging aid for the service-to-session launch path.
func runNotifyTest(kind string, minutes int) error {
	notice, err := notify.NewNotice(kind, minutes)
	if err != nil {
		return err
	}
	source, err := session.NewSessionSource()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sessions, err := source.Sessions(ctx)
	if err != nil {
		return err
	}

	notifier := notify.New()
	sent := 0
	for _, sess := range sessions {
		if sess.ID == "" || !session.IsCountedSession(sess) || !sess.Active || sess.Locked {
			continue
		}
		if err := notifier.Notify(ctx, sess, notice); err != nil {
			log.Printf("WARN: notify user=%q session=%s: %v", sess.User, sess.ID, err)
			continue
		}
		log.Printf("NOTIFY: user=%q session=%s kind=%s minutes=%d", sess.User, sess.ID, kind, minutes)
		sent++
	}
	if sent == 0 {
		return fmt.Errorf("no active local sessions found")
	}
	return nil
}

// runSelfTest performs the startup checks and exits with a non-zero status on
// failure. It backs the -selftest flag, which installers use to validate a
// deployment before registering the service.
func runSelfTest(listen string) {
	cache := process.NewCache()
	lister := process.NewLister()
	metricsServer := metrics.NewServer(listen, cache)

	if err := selftest.Run(selftest.Config{
		Privileged: isPrivileged(),
		List:       lister.List,
		Listen:     metricsServer.Listen,
	}); err != nil {
		log.Fatalf("self-test failed: %v", err)
	}

	log.Println("self-test passed")
}
