package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"timelord/internal/metrics"
	"timelord/internal/process"
	"timelord/internal/scheduler"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:9220", "address for the metrics endpoint")
	flag.Parse()

	log.Println("TimeLord starting...")

	if os.Geteuid() != 0 {
		log.Println("warning: TimeLord is not running as root, so processes of other users are not visible")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cache := process.NewCache()

	metricsServer := metrics.NewServer(*listen, cache)
	go metricsServer.Run(ctx)

	sched := scheduler.New(process.NewLister(), cache)
	sched.Run(ctx)

	log.Println("TimeLord stopped")
}
