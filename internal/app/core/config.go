package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultGRPCPort     = ":50051"
	DefaultWebPort      = ":50052"
	DefaultRealtimePort = ":8081"
	DefaultRuntimeAddr  = "localhost:50053"
	DefaultArenaWidth   = 800
	DefaultArenaHeight  = 600
	DefaultTickRate     = 60
	DefaultDBPath       = "codearena.db"
	DefaultRedisAddr    = "localhost:6379"
	DefaultGameServer   = "localhost:50051"

	botImageEnvPrefix = "CODEARENA_BOT_IMAGE_"
)

// Config is the shared configuration for every engine entrypoint. Standalone
// binaries, the unified CLI and tests all build this struct so defaults and
// validation cannot drift between them.
type Config struct {
	GRPCPort string
	WebPort  string

	RunRealtime  bool
	RealtimePort string

	RuntimeAddr string

	ArenaWidth  float32
	ArenaHeight float32
	TickRate    int
	RedisAddr   string
	DBPath      string

	// BotImage is used for runtime-managed bots. It is opt-in because the
	// RegisterBot proto has no image field. Language-specific images can be set
	// with CODEARENA_BOT_IMAGE_<LANGUAGE> (for example _GO or _PYTHON).
	BotImage   string
	BotImages  map[string]string
	GameServer string
}

// ConfigFromEnv builds the engine config from the same environment variables
// accepted by the CLI (CODEARENA_* with legacy aliases for the standalone
// binary). Invalid numeric values fail at startup instead of being silently
// replaced with defaults.
func ConfigFromEnv() (Config, error) {
	get := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value
			}
		}
		return ""
	}

	cfg := Config{
		GRPCPort:     get("CODEARENA_GRPC_PORT", "GRPC_PORT"),
		WebPort:      get("CODEARENA_WEB_PORT", "WEB_PORT"),
		RealtimePort: get("CODEARENA_REALTIME_PORT", "REALTIME_PORT"),
		RuntimeAddr:  get("CODEARENA_RUNTIME_ADDR", "RUNTIME_ADDR"),
		RunRealtime:  get("CODEARENA_NO_REALTIME", "NO_REALTIME") != "true",
		RedisAddr:    get("CODEARENA_REDIS_ADDR", "REDIS_ADDR"),
		DBPath:       get("CODEARENA_DB_PATH", "DB_PATH"),
		BotImage:     get("CODEARENA_BOT_IMAGE"),
		GameServer:   get("CODEARENA_GAME_SERVER_URL", "GAME_SERVER_URL"),
	}

	var err error
	if raw := get("CODEARENA_ARENA_WIDTH", "ARENA_WIDTH"); raw != "" {
		cfg.ArenaWidth, err = parseFloat(raw, "arena width")
		if err != nil {
			return Config{}, err
		}
	}
	if raw := get("CODEARENA_ARENA_HEIGHT", "ARENA_HEIGHT"); raw != "" {
		cfg.ArenaHeight, err = parseFloat(raw, "arena height")
		if err != nil {
			return Config{}, err
		}
	}
	if raw := get("CODEARENA_TICK_RATE", "TICK_RATE"); raw != "" {
		cfg.TickRate, err = parseInt(raw, "tick rate")
		if err != nil {
			return Config{}, err
		}
	}

	// The standalone engine binary historically always expected a runtime.
	if cfg.RuntimeAddr == "" {
		cfg.RuntimeAddr = DefaultRuntimeAddr
	}
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// WithDefaults fills only zero values; a negative or otherwise invalid value
// is left untouched so Validate can reject it.
func (c Config) WithDefaults() Config {
	c.GRPCPort = listenAddress(orDefault(c.GRPCPort, DefaultGRPCPort))
	c.WebPort = listenAddress(orDefault(c.WebPort, DefaultWebPort))
	c.RealtimePort = listenAddress(orDefault(c.RealtimePort, DefaultRealtimePort))
	// An empty address means direct-connect mode: no runtime client is created.
	c.RuntimeAddr = dialAddress(strings.TrimSpace(c.RuntimeAddr))
	c.ArenaWidth = orDefaultFloat(c.ArenaWidth, DefaultArenaWidth)
	c.ArenaHeight = orDefaultFloat(c.ArenaHeight, DefaultArenaHeight)
	c.TickRate = orDefaultInt(c.TickRate, DefaultTickRate)
	c.DBPath = orDefault(c.DBPath, DefaultDBPath)
	c.RedisAddr = orDefault(c.RedisAddr, DefaultRedisAddr)
	c.GameServer = orDefault(c.GameServer, DefaultGameServer)

	if c.BotImage == "" {
		c.BotImage = strings.TrimSpace(os.Getenv("CODEARENA_BOT_IMAGE"))
	}
	if len(c.BotImages) == 0 {
		c.BotImages = botImagesFromEnv()
	}
	return c
}

// Validate rejects values that cannot produce a usable engine. It must be
// called after WithDefaults.
func (c Config) Validate() error {
	if c.ArenaWidth <= 0 || c.ArenaHeight <= 0 {
		return fmt.Errorf("arena dimensions must be positive (got %gx%g)", c.ArenaWidth, c.ArenaHeight)
	}
	if c.ArenaWidth > 100000 || c.ArenaHeight > 100000 {
		return fmt.Errorf("arena dimensions are unrealistically large (got %gx%g)", c.ArenaWidth, c.ArenaHeight)
	}
	if c.TickRate <= 0 || c.TickRate > 240 {
		return fmt.Errorf("tick rate must be between 1 and 240 (got %d)", c.TickRate)
	}
	if c.GRPCPort == "" || c.WebPort == "" {
		return fmt.Errorf("grpc and web ports are required")
	}
	if strings.TrimSpace(c.DBPath) == "" {
		return fmt.Errorf("database path is required")
	}
	return nil
}

// BotImageForLanguage returns the configured image for a bot language, falling
// back to the global image. An empty string means bots are expected to connect
// directly and no runtime container should be started.
func (c Config) BotImageForLanguage(language string) string {
	key := strings.ToLower(strings.TrimSpace(language))
	if image := c.BotImages[key]; image != "" {
		return image
	}
	return c.BotImage
}

func botImagesFromEnv() map[string]string {
	images := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" || !strings.HasPrefix(key, botImageEnvPrefix) {
			continue
		}
		language := strings.ToLower(strings.TrimPrefix(key, botImageEnvPrefix))
		if language != "" {
			images[language] = value
		}
	}
	return images
}

func listenAddress(addr string) string {
	if addr == "" || strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

func dialAddress(addr string) string {
	if addr != "" && !strings.Contains(addr, ":") {
		return "localhost:" + addr
	}
	return addr
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func orDefaultFloat(value, fallback float32) float32 {
	if value == 0 {
		return fallback
	}
	return value
}

func orDefaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func parseFloat(raw, name string) (float32, error) {
	value, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, raw, err)
	}
	return float32(value), nil
}

func parseInt(raw, name string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, raw, err)
	}
	return value, nil
}
