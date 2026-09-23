package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"timelord/internal/process"
	"timelord/internal/scheduler"
)

func main() {
	log.Println("TimeLord starting...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cache := process.NewCache()
	sched := scheduler.New(process.NewLister(), cache)
	sched.Run(ctx)

	log.Println("TimeLord stopped")
}
