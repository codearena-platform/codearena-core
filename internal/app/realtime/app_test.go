package realtime

import (
	"context"
	"errors"
	"testing"
)

func TestStartWithContextPassesConfig(t *testing.T) {
	var got Config
	original := startRealtime
	startRealtime = func(_ context.Context, port, engineAddr, redisAddr string) error {
		got = Config{Port: port, EngineAddr: engineAddr, RedisAddr: redisAddr}
		return nil
	}
	t.Cleanup(func() { startRealtime = original })

	want := Config{Port: ":9000", EngineAddr: "engine:50051", RedisAddr: "redis:6379"}
	if err := StartWithContext(context.Background(), want); err != nil {
		t.Fatalf("StartWithContext: %v", err)
	}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestStartPropagatesError(t *testing.T) {
	want := errors.New("realtime failed")
	original := startRealtime
	startRealtime = func(context.Context, string, string, string) error { return want }
	t.Cleanup(func() { startRealtime = original })

	if err := StartWithContext(context.Background(), Config{}); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
