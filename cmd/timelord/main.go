package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"timelord/internal/metrics"
	"timelord/internal/process"
	"timelord/internal/scheduler"
	"timelord/internal/selftest"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:9220", "address for the metrics endpoint")
	selfTestOnly := flag.Bool("selftest", false, "run the startup self-test and exit")
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
		sched.Run(ctx)

		log.Println("TimeLord stopped")
		return nil
	})
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
