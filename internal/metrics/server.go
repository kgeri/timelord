package metrics

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"timelord/internal/process"
)

// shutdownTimeout is the time given to open requests at shutdown.
const shutdownTimeout = 5 * time.Second

// Server serves the Prometheus metrics endpoint.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

// NewServer returns a server that exposes the process cache on addr at /metrics.
func NewServer(addr string, cache *process.Cache) *Server {
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewCollector(cache))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
}

// Listen opens the metrics endpoint. It is safe to call more than once; later
// calls reuse the first listener.
//
// The self-test calls Listen before Run so that a port conflict fails startup
// instead of being logged and ignored by the server goroutine.
func (s *Server) Listen() error {
	if s.listener != nil {
		return nil
	}

	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	s.listener = listener
	return nil
}

// Run serves requests until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	if err := s.Listen(); err != nil {
		log.Printf("metrics server failed: %v", err)
		return
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("metrics server shutdown: %v", err)
		}
	}()

	log.Printf("metrics listening on %s", s.listener.Addr())
	if err := s.httpServer.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("metrics server failed: %v", err)
	}
}
