package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"timelord/internal/enforcer"
	"timelord/internal/metrics"
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
	selfTestOnly := flag.Bool("selftest", false, "run the startup self-test and exit")
	var users userList
	flag.Var(&users, "user", "account to manage; repeat the flag for multiple users")
	flag.Parse()

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
		enforcerKey := os.Getenv("TIMELORD_ENFORCER_KEY")
		if enforcerKey == "" {
			return fmt.Errorf("TIMELORD_ENFORCER_KEY is not set")
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

// userList collects a repeatable -user flag.
type userList []string

func (u *userList) String() string {
	return strings.Join(*u, ",")
}

func (u *userList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("user must not be empty")
	}
	*u = append(*u, value)
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
