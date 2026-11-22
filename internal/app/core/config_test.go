package core

import (
	"strings"
	"testing"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	cfg := Config{}.WithDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
	if cfg.GRPCPort != DefaultGRPCPort || cfg.WebPort != DefaultWebPort || cfg.RealtimePort != DefaultRealtimePort {
		t.Errorf("unexpected ports: %+v", cfg)
	}
	if cfg.ArenaWidth != DefaultArenaWidth || cfg.ArenaHeight != DefaultArenaHeight || cfg.TickRate != DefaultTickRate {
		t.Errorf("unexpected simulation defaults: %+v", cfg)
	}
	if cfg.RuntimeAddr != "" {
		t.Errorf("empty runtime address should remain disabled, got %q", cfg.RuntimeAddr)
	}
	if cfg.GameServer != DefaultGameServer {
		t.Errorf("game server = %q, want %q", cfg.GameServer, DefaultGameServer)
	}
}

func TestConfigValidatesNegativeDimensionsAndTickRate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"negative width", Config{ArenaWidth: -1}, "arena dimensions"},
		{"negative height", Config{ArenaHeight: -1}, "arena dimensions"},
		{"negative tick rate", Config{TickRate: -1}, "tick rate"},
		{"excessive tick rate", Config{TickRate: 241}, "tick rate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg.WithDefaults()
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("CODEARENA_GRPC_PORT", "9090")
	t.Setenv("CODEARENA_TICK_RATE", "30")
	t.Setenv("CODEARENA_ARENA_WIDTH", "1024")
	t.Setenv("CODEARENA_NO_REALTIME", "true")
	t.Setenv("CODEARENA_BOT_IMAGE", "bot:latest")
	t.Setenv("CODEARENA_BOT_IMAGE_PYTHON", "bot-python:latest")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.GRPCPort != ":9090" {
		t.Errorf("grpc port = %q, want :9090", cfg.GRPCPort)
	}
	if cfg.TickRate != 30 || cfg.ArenaWidth != 1024 {
		t.Errorf("unexpected simulation config: %+v", cfg)
	}
	if cfg.RunRealtime {
		t.Error("NO_REALTIME=true should disable embedded realtime")
	}
	if cfg.RuntimeAddr != DefaultRuntimeAddr {
		t.Errorf("runtime addr = %q, want default %q", cfg.RuntimeAddr, DefaultRuntimeAddr)
	}
	if got := cfg.BotImageForLanguage("Python"); got != "bot-python:latest" {
		t.Errorf("python image = %q, want bot-python:latest", got)
	}
	if got := cfg.BotImageForLanguage("go"); got != "bot:latest" {
		t.Errorf("go image = %q, want bot:latest", got)
	}
}

func TestConfigFromEnvRejectsInvalidNumbers(t *testing.T) {
	t.Setenv("CODEARENA_TICK_RATE", "fast")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("ConfigFromEnv accepted a non-numeric tick rate")
	}
}
