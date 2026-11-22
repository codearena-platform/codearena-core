package routes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Servers owns the engine's gRPC and dashboard HTTP servers so callers can
// observe serve errors and shut both down deterministically.
type Servers struct {
	GRPC       *grpc.Server
	HTTP       *http.Server
	Simulation *SimulationServer

	mu       sync.Mutex
	shutdown bool
}

func NewServers(e *services.SimulationEngine, tickRate time.Duration) *Servers {
	srv := NewSimulationServer(e, tickRate)
	grpcSrv := grpc.NewServer()
	pb.RegisterBotServiceServer(grpcSrv, srv)
	pb.RegisterSimulationServiceServer(grpcSrv, srv)
	pb.RegisterMatchServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.HandleDashboardWS)

	return &Servers{GRPC: grpcSrv, HTTP: newWebServer(mux), Simulation: srv}
}

func newWebServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// Serve blocks until either server fails or Shutdown is called, then stops both.
func (s *Servers) Serve(grpcAddr, webAddr string) error {
	// A shutdown may be requested before Serve gets a chance to bind (for
	// example when the parent context is already canceled). Do not bind
	// listeners after Shutdown has run.
	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("gRPC listener failed on %s: %w", grpcAddr, err)
	}
	webLis, err := net.Listen("tcp", webAddr)
	if err != nil {
		grpcLis.Close()
		return fmt.Errorf("web listener failed on %s: %w", webAddr, err)
	}
	defer webLis.Close()
	defer s.GRPC.Stop()
	defer s.HTTP.Close()

	s.mu.Lock()
	if s.shutdown {
		s.mu.Unlock()
		grpcLis.Close()
		webLis.Close()
		return nil
	}
	s.mu.Unlock()

	errCh := make(chan error, 2)
	go func() {
		err := s.GRPC.Serve(grpcLis)
		if err != nil {
			err = fmt.Errorf("gRPC server failed: %w", err)
		}
		errCh <- err // also unblocks Serve after a graceful Stop
	}()
	go func() {
		if err := s.HTTP.Serve(webLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("web server failed: %w", err)
		}
	}()

	log.Printf("GRPC Server listening at %s", grpcAddr)
	log.Printf("Web Dashboard Data (WS) available at ws://localhost%s/ws", webAddr)
	return <-errCh
}

// Shutdown stops the simulation loop, then drains gRPC with a bounded
// GracefulStop and shuts the HTTP server down. If the context expires before
// in-flight RPCs finish, the gRPC server is force-stopped.
func (s *Servers) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()

	if s.Simulation != nil {
		s.Simulation.Shutdown()
	}

	stopped := make(chan struct{})
	go func() {
		s.GRPC.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-ctx.Done():
		s.GRPC.Stop()
		<-stopped
	}

	if err := s.HTTP.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("web server shutdown failed: %w", err)
	}
	return nil
}

// StartAll is kept as a small wrapper for callers that do not need lifecycle control.
func StartAll(grpcAddr, webAddr string, e *services.SimulationEngine, tickRate time.Duration) error {
	return NewServers(e, tickRate).Serve(grpcAddr, webAddr)
}
