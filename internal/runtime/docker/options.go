package docker

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

// SandboxConfig describes the restrictions applied to every bot container.
// Defaults are secure; only explicit environment variables may relax them.
type SandboxConfig struct {
	MemoryBytes      int64
	MemorySwapBytes  int64
	NanoCPUs         int64
	PidsLimit        int64
	NetworkMode      container.NetworkMode
	ReadonlyRootfs   bool
	User             string
	CapDrop          []string
	SecurityOpt      []string
	Tmpfs            map[string]string
	StartTimeout     time.Duration
	StopTimeout      time.Duration
	MaxExecutionTime time.Duration
	AllowedImages    []string
	RequireDigest    bool
}

const (
	botUser = "65532:65532" // nobody:nogroup, numeric so it works with distroless images
	tmpfs   = "rw,noexec,nosuid,size=64m,mode=1777"
)

// DefaultSandboxConfig returns the hardened defaults used when no overrides are set.
func DefaultSandboxConfig() SandboxConfig {
	memory := int64(512 * 1024 * 1024)
	return SandboxConfig{
		MemoryBytes:      memory,
		MemorySwapBytes:  memory, // memory + swap == memory disables swap
		NanoCPUs:         500_000_000,
		PidsLimit:        64,
		NetworkMode:      container.NetworkMode("none"),
		ReadonlyRootfs:   true,
		User:             botUser,
		CapDrop:          []string{"ALL"},
		SecurityOpt:      []string{"no-new-privileges:true"}, // Docker's default seccomp profile stays enabled
		Tmpfs:            map[string]string{"/tmp": tmpfs},
		StartTimeout:     2 * time.Minute,
		StopTimeout:      5 * time.Second,
		MaxExecutionTime: 5 * time.Minute,
	}
}

// SandboxConfigFromEnv loads sandbox limits from the environment.
func SandboxConfigFromEnv() (SandboxConfig, error) {
	cfg := DefaultSandboxConfig()
	var err error

	if cfg.MemoryBytes, err = envInt64("RUNTIME_BOT_MEMORY_BYTES", cfg.MemoryBytes); err != nil {
		return SandboxConfig{}, err
	}
	swapOverride := strings.TrimSpace(os.Getenv("RUNTIME_BOT_MEMORY_SWAP_BYTES"))
	if cfg.MemorySwapBytes, err = envInt64("RUNTIME_BOT_MEMORY_SWAP_BYTES", cfg.MemorySwapBytes); err != nil {
		return SandboxConfig{}, err
	}
	if swapOverride == "" && cfg.MemorySwapBytes > cfg.MemoryBytes {
		cfg.MemorySwapBytes = cfg.MemoryBytes // keep swap disabled when only memory is lowered
	}
	if cfg.NanoCPUs, err = envInt64("RUNTIME_BOT_NANO_CPUS", cfg.NanoCPUs); err != nil {
		return SandboxConfig{}, err
	}
	if cfg.PidsLimit, err = envInt64("RUNTIME_BOT_PIDS_LIMIT", cfg.PidsLimit); err != nil {
		return SandboxConfig{}, err
	}
	if cfg.StartTimeout, err = envDuration("RUNTIME_BOT_START_TIMEOUT", cfg.StartTimeout); err != nil {
		return SandboxConfig{}, err
	}
	if cfg.StopTimeout, err = envDuration("RUNTIME_BOT_STOP_TIMEOUT", cfg.StopTimeout); err != nil {
		return SandboxConfig{}, err
	}
	if cfg.MaxExecutionTime, err = envDuration("RUNTIME_BOT_TIMEOUT", cfg.MaxExecutionTime); err != nil {
		return SandboxConfig{}, err
	}

	if mode := strings.TrimSpace(os.Getenv("RUNTIME_BOT_NETWORK")); mode != "" {
		cfg.NetworkMode = container.NetworkMode(mode)
	}
	if user := strings.TrimSpace(os.Getenv("RUNTIME_BOT_USER")); user != "" {
		cfg.User = user
	}
	if profile := strings.TrimSpace(os.Getenv("RUNTIME_SECCOMP_PROFILE")); profile != "" {
		cfg.SecurityOpt = append(cfg.SecurityOpt, "seccomp="+profile)
	}
	if value := strings.TrimSpace(os.Getenv("RUNTIME_READONLY_ROOTFS")); value != "" {
		readonly, err := strconv.ParseBool(value)
		if err != nil {
			return SandboxConfig{}, fmt.Errorf("invalid RUNTIME_READONLY_ROOTFS: %w", err)
		}
		cfg.ReadonlyRootfs = readonly
	}

	cfg.AllowedImages = splitList(os.Getenv("RUNTIME_ALLOWED_IMAGES"))
	cfg.RequireDigest = len(cfg.AllowedImages) > 0
	if value := strings.TrimSpace(os.Getenv("RUNTIME_REQUIRE_DIGEST")); value != "" {
		cfg.RequireDigest, err = strconv.ParseBool(value)
		if err != nil {
			return SandboxConfig{}, fmt.Errorf("invalid RUNTIME_REQUIRE_DIGEST: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return SandboxConfig{}, err
	}
	return cfg, nil
}

// Validate rejects configuration that would disable the sandbox limits.
func (c SandboxConfig) Validate() error {
	if c.MemoryBytes <= 0 || c.MemorySwapBytes <= 0 || c.MemorySwapBytes > c.MemoryBytes {
		return fmt.Errorf("memory limits must be positive and swap must not exceed memory")
	}
	if c.NanoCPUs <= 0 || c.PidsLimit <= 0 {
		return fmt.Errorf("CPU and PID limits must be positive")
	}
	if c.User == "" {
		return fmt.Errorf("bot user must be set")
	}
	if len(c.CapDrop) == 0 || len(c.SecurityOpt) == 0 {
		return fmt.Errorf("capability drop and security options are required")
	}
	if c.StartTimeout <= 0 || c.StopTimeout <= 0 || c.MaxExecutionTime <= 0 {
		return fmt.Errorf("start, stop and execution timeouts must be positive")
	}
	return nil
}

// ContainerConfig builds the immutable container configuration for a bot.
func (c SandboxConfig) ContainerConfig(image, botID string, env []string) *container.Config {
	stopTimeout := int(c.StopTimeout.Seconds())
	return &container.Config{
		Image:           image,
		Env:             env,
		User:            c.User,
		NetworkDisabled: c.NetworkMode == "none",
		StopTimeout:     &stopTimeout,
		Labels: map[string]string{
			"codearena.bot.id": botID,
			"managed_by":       "codearena-runtime",
		},
	}
}

// HostConfig builds the resource limits and isolation options for a bot container.
func (c SandboxConfig) HostConfig() *container.HostConfig {
	pids := c.PidsLimit
	return &container.HostConfig{
		AutoRemove:     true,
		ReadonlyRootfs: c.ReadonlyRootfs,
		NetworkMode:    c.NetworkMode,
		CapDrop:        append([]string(nil), c.CapDrop...),
		SecurityOpt:    append([]string(nil), c.SecurityOpt...),
		Tmpfs:          copyMap(c.Tmpfs),
		Resources: container.Resources{
			NanoCPUs:   c.NanoCPUs,
			Memory:     c.MemoryBytes,
			MemorySwap: c.MemorySwapBytes,
			PidsLimit:  &pids,
		},
	}
}

// ValidateImage enforces the optional image allowlist and digest pinning.
func (c SandboxConfig) ValidateImage(image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is required")
	}
	if len(c.AllowedImages) > 0 {
		allowed := false
		for _, candidate := range c.AllowedImages {
			if image == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("image %q is not in the runtime allowlist", image)
		}
	}
	if c.RequireDigest && !strings.Contains(image, "@sha256:") {
		return fmt.Errorf("image %q must be pinned by sha256 digest", image)
	}
	return nil
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func copyMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func envInt64(name string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}
