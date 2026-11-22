package realtime

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewRealtimeHTTPServerTimeouts(t *testing.T) {
	srv := newRealtimeHTTPServer(":0", http.NewServeMux())
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatalf("HTTP server is missing timeouts: %+v", srv)
	}
	if srv.Handler == nil {
		t.Fatal("expected an explicit handler instead of DefaultServeMux")
	}
}

func TestHubRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hub := newHub(nil)
	go hub.run(ctx)

	cancel()
	select {
	case <-hub.done:
	case <-time.After(time.Second):
		t.Fatal("hub did not stop when its context was canceled")
	}

	// Sends from WebSocket handlers must not block after the hub has stopped.
	done := make(chan struct{})
	go func() {
		select {
		case hub.register <- &Client{hub: hub, send: make(chan []byte), matchID: "m"}:
		case <-hub.done:
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("register send blocked after hub shutdown")
	}
}

func TestSleepCtxStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, time.Hour) {
		t.Fatal("expected canceled context to interrupt sleep")
	}
}
