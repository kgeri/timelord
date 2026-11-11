package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"timelord/internal/timetower"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:8080", "address for the Timetower API")
	configPath := flag.String("config", "timetower.json", "path to the Timetower configuration file")
	statePath := flag.String("state", "timetower-state.json", "path to the persisted usage and status state")
	flag.Parse()

	config, err := timetower.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("timetower: load config: %v", err)
	}
	for i := range config.Users {
		log.Printf("timetower: user %q", config.Users[i].Name)
	}

	key := os.Getenv("TIMELORD_KEY")
	if key == "" {
		log.Fatal("timetower: TIMELORD_KEY is not set")
	}

	store, err := timetower.NewStore(*statePath)
	if err != nil {
		log.Fatalf("timetower: load state: %v", err)
	}

	// Keep the documented 400-day usage window.
	cutoff := time.Now().In(config.Location()).AddDate(0, 0, -400).Format("2006-01-02")
	store.PruneUsage(cutoff)

	api := timetower.NewServer(config, store, key)
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go api.Monitor(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Printf("timetower listening on %s", *listen)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		log.Fatalf("timetower: %v", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("timetower: shutdown: %v", err)
	}
	if err := store.Save(); err != nil {
		log.Printf("timetower: save state: %v", err)
	}
	log.Println("timetower stopped")
}
