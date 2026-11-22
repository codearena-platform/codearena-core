package runtime

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
)

type acceptSignalListener struct {
	net.Listener
	accepted chan struct{}
	once     sync.Once
}

func (l *acceptSignalListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.accepted) })
	return l.Listener.Accept()
}

func TestGracefulStopDrainsServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	signal := &acceptSignalListener{Listener: listener, accepted: make(chan struct{})}

	server := grpc.NewServer()
	served := make(chan error, 1)
	go func() { served <- server.Serve(signal) }()

	select {
	case <-signal.accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server never started accepting connections")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gracefulStop(server, ctx)

	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("gRPC Serve returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server did not stop")
	}
}

func TestGracefulStopReturnsAfterDeadline(t *testing.T) {
	server := grpc.NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		gracefulStop(server, ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("gracefulStop did not force-stop the server after context cancellation")
	}
}
