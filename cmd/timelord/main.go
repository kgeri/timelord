package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/user"
	"path/filepath"
	"strings"

	"timelord/internal/enforce"
	"timelord/internal/metrics"
	"timelord/internal/process"
	"timelord/internal/schedule"
	"timelord/internal/scheduler"
	"timelord/internal/selftest"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:9220", "address for the metrics endpoint")
	selfTestOnly := flag.Bool("selftest", false, "run the startup self-test and exit")
	var users userList
	flag.Var(&users, "user", "user to restrict; repeat the flag for multiple users")
	flag.Parse()

	if *selfTestOnly {
		runSelfTest(*listen)
		return
	}

	runPlatform(func(ctx context.Context) error {
		log.Println("TimeLord starting...")

		files := resolveScheduleFiles(users)
		store := schedule.NewStore(fileUsers(files))
		provider := schedule.NewFileProvider(files)

		config, err := provider.Load()
		if err != nil {
			return fmt.Errorf("load schedule: %w", err)
		}
		store.Replace(config)
		logSchedule(store)

		var engine *enforce.Engine
		var usageSource metrics.UsageSource
		sessionSource, err := enforce.NewSessionSource()
		if err != nil {
			log.Printf("WARN: schedule enforcement disabled: %v", err)
		} else {
			engine = enforce.NewEngine(sessionSource, store)
			usageSource = engine
		}

		cache := process.NewCache()
		lister := process.NewLister()
		metricsServer := metrics.NewServer(*listen, cache, usageSource)

		if err := selftest.Run(selftest.Config{
			Privileged: isPrivileged(),
			List:       lister.List,
			Listen:     metricsServer.Listen,
		}); err != nil {
			return fmt.Errorf("self-test failed: %w", err)
		}

		go provider.Watch(ctx, func(config *schedule.Config, err error) {
			if err != nil {
				log.Printf("WARN: schedule reload failed: %v", err)
				return
			}
			store.Replace(config)
			log.Println("schedule reloaded")
			logSchedule(store)
		})
		if engine != nil && len(store.Users()) > 0 {
			go engine.Run(ctx)
		}
		go metricsServer.Run(ctx)

		sched := scheduler.New(lister, cache)
		sched.Run(ctx)

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

// resolveScheduleFiles finds the home directory and schedule file for each
// managed user. A missing user is a warning, not a startup failure.
func resolveScheduleFiles(users []string) []schedule.File {
	if len(users) == 0 {
		log.Println("WARN: no -user option given: schedule enforcement is disabled")
		return nil
	}

	files := make([]schedule.File, 0, len(users))
	seen := make(map[string]bool, len(users))
	for _, name := range users {
		if seen[name] {
			continue
		}
		seen[name] = true

		account, err := user.Lookup(name)
		if err != nil {
			log.Printf("WARN: schedule: user %q not found: %v", name, err)
			continue
		}
		if account.HomeDir == "" {
			log.Printf("WARN: schedule: user %q has no home directory", name)
			continue
		}

		path := filepath.Join(account.HomeDir, schedule.FileName)
		log.Printf("schedule: managing user %s from %s", name, path)
		files = append(files, schedule.File{User: name, Path: path})
	}
	return files
}

// fileUsers returns the user names from files.
func fileUsers(files []schedule.File) []string {
	users := make([]string, 0, len(files))
	for _, file := range files {
		users = append(users, file.User)
	}
	return users
}

// logSchedule writes the currently loaded policies.
func logSchedule(store *schedule.Store) {
	for _, user := range store.Users() {
		policy, ok := store.Policy(user)
		if !ok {
			log.Printf("schedule: user %s has no policy: no time allowed", user)
			continue
		}
		if policy.DailyLimitMinutes == 0 {
			log.Printf("schedule: user %s, %d window(s), no daily limit", user, len(policy.Windows))
			continue
		}
		log.Printf("schedule: user %s, %d window(s), daily limit %d minute(s)",
			user, len(policy.Windows), policy.DailyLimitMinutes)
	}
}

// runSelfTest performs the startup checks and exits with a non-zero status on
// failure. It backs the -selftest flag, which installers use to validate a
// deployment before registering the service.
func runSelfTest(listen string) {
	cache := process.NewCache()
	lister := process.NewLister()
	metricsServer := metrics.NewServer(listen, cache, nil)

	if err := selftest.Run(selftest.Config{
		Privileged: isPrivileged(),
		List:       lister.List,
		Listen:     metricsServer.Listen,
	}); err != nil {
		log.Fatalf("self-test failed: %v", err)
	}

	log.Println("self-test passed")
}
