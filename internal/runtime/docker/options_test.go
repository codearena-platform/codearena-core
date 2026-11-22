package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
)

func clearSandboxEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"RUNTIME_BOT_MEMORY_BYTES", "RUNTIME_BOT_MEMORY_SWAP_BYTES",
		"RUNTIME_BOT_NANO_CPUS", "RUNTIME_BOT_PIDS_LIMIT",
		"RUNTIME_BOT_NETWORK", "RUNTIME_BOT_USER", "RUNTIME_SECCOMP_PROFILE",
		"RUNTIME_READONLY_ROOTFS", "RUNTIME_BOT_START_TIMEOUT",
		"RUNTIME_BOT_STOP_TIMEOUT", "RUNTIME_BOT_TIMEOUT",
		"RUNTIME_ALLOWED_IMAGES", "RUNTIME_REQUIRE_DIGEST",
	} {
		t.Setenv(key, "")
	}
}

func TestDefaultSandboxConfigIsHardened(t *testing.T) {
	clearSandboxEnv(t)
	cfg, err := SandboxConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.NetworkMode != "none" {
		t.Errorf("expected network disabled, got %q", cfg.NetworkMode)
	}
	if !cfg.ReadonlyRootfs {
		t.Error("expected read-only rootfs")
	}
	if cfg.User == "" || cfg.User == "0" || cfg.User == "root" {
		t.Errorf("expected non-root user, got %q", cfg.User)
	}
	if len(cfg.CapDrop) != 1 || cfg.CapDrop[0] != "ALL" {
		t.Errorf("expected all capabilities dropped, got %v", cfg.CapDrop)
	}
	if len(cfg.SecurityOpt) == 0 || cfg.SecurityOpt[0] != "no-new-privileges:true" {
		t.Errorf("expected no-new-privileges, got %v", cfg.SecurityOpt)
	}
	if cfg.MemoryBytes <= 0 || cfg.MemorySwapBytes > cfg.MemoryBytes {
		t.Errorf("invalid memory limits: memory=%d swap=%d", cfg.MemoryBytes, cfg.MemorySwapBytes)
	}
	if cfg.NanoCPUs <= 0 || cfg.PidsLimit <= 0 {
		t.Errorf("invalid cpu/pids limits: %d / %d", cfg.NanoCPUs, cfg.PidsLimit)
	}
	if cfg.MaxExecutionTime <= 0 || cfg.StartTimeout <= 0 || cfg.StopTimeout <= 0 {
		t.Errorf("missing timeouts: %+v", cfg)
	}
	if cfg.RequireDigest {
		t.Error("digest requirement should be opt-in when no allowlist is configured")
	}
}

func TestSandboxConfigEnvOverridesAndErrors(t *testing.T) {
	clearSandboxEnv(t)
	t.Setenv("RUNTIME_BOT_MEMORY_BYTES", "268435456")
	t.Setenv("RUNTIME_BOT_NANO_CPUS", "250000000")
	t.Setenv("RUNTIME_BOT_NETWORK", "custom-net")
	t.Setenv("RUNTIME_SECCOMP_PROFILE", "/etc/seccomp/bot.json")
	t.Setenv("RUNTIME_BOT_TIMEOUT", "30s")
	t.Setenv("RUNTIME_READONLY_ROOTFS", "false")

	cfg, err := SandboxConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MemoryBytes != 268435456 || cfg.NanoCPUs != 250000000 || cfg.MemorySwapBytes != cfg.MemoryBytes {
		t.Errorf("unexpected limits: %+v", cfg)
	}
	if cfg.NetworkMode != "custom-net" || cfg.ReadonlyRootfs {
		t.Errorf("unexpected isolation settings: %+v", cfg)
	}
	if cfg.MaxExecutionTime != 30*time.Second {
		t.Errorf("unexpected execution deadline: %v", cfg.MaxExecutionTime)
	}
	if len(cfg.SecurityOpt) != 2 || cfg.SecurityOpt[1] != "seccomp=/etc/seccomp/bot.json" {
		t.Errorf("unexpected security options: %v", cfg.SecurityOpt)
	}

	t.Setenv("RUNTIME_BOT_NANO_CPUS", "not-a-number")
	if _, err := SandboxConfigFromEnv(); err == nil {
		t.Fatal("expected error for invalid numeric limit")
	}
}

func TestValidateImageAllowlistAndDigest(t *testing.T) {
	tests := []struct {
		name    string
		allowed string
		digest  string
		image   string
		wantErr bool
	}{
		{"no allowlist accepts any image", "", "", "codearena/bot-go:latest", false},
		{"allowlist exact match with digest", "codearena/bot-go@sha256:abc", "", "codearena/bot-go@sha256:abc", false},
		{"allowlist rejects unknown image", "codearena/bot-go@sha256:abc", "", "evil/image@sha256:abc", true},
		{"allowlist requires digest", "codearena/bot-go:latest", "", "codearena/bot-go:latest", true},
		{"digest can be required without allowlist", "", "true", "codearena/bot-go:latest", true},
		{"empty image rejected", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSandboxEnv(t)
			t.Setenv("RUNTIME_ALLOWED_IMAGES", tt.allowed)
			t.Setenv("RUNTIME_REQUIRE_DIGEST", tt.digest)
			cfg, err := SandboxConfigFromEnv()
			if err != nil {
				t.Fatalf("unexpected config error: %v", err)
			}
			err = cfg.ValidateImage(tt.image)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateImage(%q) error=%v, wantErr=%v", tt.image, err, tt.wantErr)
			}
		})
	}
}

func TestContainerAndHostConfigAreHardened(t *testing.T) {
	cfg := DefaultSandboxConfig()
	containerCfg := cfg.ContainerConfig("codearena/bot-go@sha256:abc", "bot-1", []string{"BOT_ID=bot-1"})
	if containerCfg.Image != "codearena/bot-go@sha256:abc" || containerCfg.User != cfg.User {
		t.Errorf("unexpected container config: %+v", containerCfg)
	}
	if !containerCfg.NetworkDisabled {
		t.Error("expected networking disabled in container config")
	}
	if containerCfg.Labels["managed_by"] != "codearena-runtime" {
		t.Errorf("missing ownership label: %v", containerCfg.Labels)
	}

	hostCfg := cfg.HostConfig()
	if !hostCfg.AutoRemove || !hostCfg.ReadonlyRootfs || hostCfg.NetworkMode != "none" {
		t.Errorf("unexpected host config: %+v", hostCfg)
	}
	if len(hostCfg.CapDrop) != 1 || hostCfg.CapDrop[0] != "ALL" {
		t.Errorf("expected all capabilities dropped, got %v", hostCfg.CapDrop)
	}
	if hostCfg.Privileged {
		t.Error("containers must not be privileged")
	}
	if hostCfg.Resources.Memory != cfg.MemoryBytes || hostCfg.Resources.MemorySwap != cfg.MemorySwapBytes {
		t.Errorf("unexpected memory limits: %+v", hostCfg.Resources)
	}
	if hostCfg.Resources.NanoCPUs != cfg.NanoCPUs || hostCfg.Resources.PidsLimit == nil || *hostCfg.Resources.PidsLimit != cfg.PidsLimit {
		t.Errorf("unexpected CPU/pids limits: %+v", hostCfg.Resources)
	}
	if _, ok := hostCfg.Tmpfs["/tmp"]; !ok {
		t.Errorf("expected writable tmpfs for read-only rootfs, got %v", hostCfg.Tmpfs)
	}

	// Mutating the returned config must not mutate the shared sandbox config.
	hostCfg.CapDrop[0] = "NET_ADMIN"
	if cfg.CapDrop[0] != "ALL" {
		t.Error("HostConfig must copy capability list")
	}
}

func TestHostConfigSeccompProfileOption(t *testing.T) {
	clearSandboxEnv(t)
	t.Setenv("RUNTIME_SECCOMP_PROFILE", "/profiles/bot.json")
	cfg, err := SandboxConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hostCfg := cfg.HostConfig()
	for _, opt := range hostCfg.SecurityOpt {
		if opt == "seccomp=/profiles/bot.json" {
			return
		}
	}
	t.Fatalf("expected custom seccomp option, got %v", hostCfg.SecurityOpt)
}

func TestSandboxConfigRejectsDisabledLimits(t *testing.T) {
	clearSandboxEnv(t)
	t.Setenv("RUNTIME_BOT_PIDS_LIMIT", "0")
	if _, err := SandboxConfigFromEnv(); err == nil {
		t.Fatal("expected zero PID limit to be rejected")
	}
	t.Setenv("RUNTIME_BOT_PIDS_LIMIT", "64")
	t.Setenv("RUNTIME_BOT_TIMEOUT", "0s")
	if _, err := SandboxConfigFromEnv(); err == nil {
		t.Fatal("expected zero execution deadline to be rejected")
	}
}

func TestHostConfigNetworkMode(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.NetworkMode = container.NetworkMode("bridge")
	if cfg.ContainerConfig("img", "bot", nil).NetworkDisabled {
		t.Error("NetworkDisabled should only be set for the none network")
	}
	if cfg.HostConfig().NetworkMode != "bridge" {
		t.Error("expected network mode override in host config")
	}
}
