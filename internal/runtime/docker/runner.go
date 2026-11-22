package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerAPI is the subset of the Docker client used by the sandbox runner. It
// keeps the security-sensitive behavior testable without a Docker daemon.
type dockerAPI interface {
	ImageInspectWithRaw(ctx context.Context, imageID string) (dockerimage.InspectResponse, []byte, error)
	ImagePull(ctx context.Context, refStr string, options dockerimage.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerKill(ctx context.Context, containerID, signal string) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

var botIDPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

type BotRunner struct {
	cli       dockerAPI
	cfg       SandboxConfig
	mu        sync.Mutex
	deadlines map[string]context.CancelFunc
}

func NewBotRunner() (*BotRunner, error) {
	cfg, err := SandboxConfigFromEnv()
	if err != nil {
		return nil, err
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return newBotRunner(cli, cfg), nil
}

func newBotRunner(cli dockerAPI, cfg SandboxConfig) *BotRunner {
	return &BotRunner{cli: cli, cfg: cfg, deadlines: make(map[string]context.CancelFunc)}
}

func (r *BotRunner) StartContainer(parent context.Context, image string, botID string, envVars []string) (string, error) {
	if err := r.cfg.ValidateImage(image); err != nil {
		return "", err
	}
	if !botIDPattern.MatchString(botID) {
		return "", fmt.Errorf("invalid bot id %q", botID)
	}

	// Bound image pulls and container creation even if the caller has no deadline.
	ctx, cancel := context.WithTimeout(parent, r.cfg.StartTimeout)
	defer cancel()

	if err := r.ensureImage(ctx, image); err != nil {
		return "", err
	}

	containerConfig := r.cfg.ContainerConfig(image, botID, envVars)
	hostConfig := r.cfg.HostConfig()
	name := "bot-" + botID

	resp, err := r.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, name)
	if err != nil {
		if !strings.Contains(err.Error(), "Conflict") {
			return "", fmt.Errorf("failed to create container: %w", err)
		}
		// A stale container from a previous run owns the name: remove it and retry.
		if rmErr := r.removeIfExists(ctx, name); rmErr != nil {
			return "", fmt.Errorf("failed to clean up conflicting container: %w", rmErr)
		}
		resp, err = r.cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, name)
		if err != nil {
			return "", fmt.Errorf("failed to create container (retry): %w", err)
		}
	}

	if err := r.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Never leave a created-but-not-started container behind.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), r.cfg.StopTimeout)
		defer cleanupCancel()
		if rmErr := r.cli.ContainerRemove(cleanupCtx, resp.ID, container.RemoveOptions{Force: true}); rmErr != nil && !client.IsErrNotFound(rmErr) {
			slog.Error("Failed to clean up container after start error", "container_id", resp.ID, "error", rmErr)
		}
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	r.trackDeadline(resp.ID)
	return resp.ID, nil
}

func (r *BotRunner) ensureImage(ctx context.Context, image string) error {
	_, _, err := r.cli.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return nil
	}
	if !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to inspect image %q: %w", image, err)
	}

	slog.Info("Pulling docker image", "image", image)
	out, err := r.cli.ImagePull(ctx, image, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(io.Discard, out); err != nil {
		return fmt.Errorf("failed to read image pull output: %w", err)
	}
	return nil
}

// trackDeadline stops the container after the configured execution deadline.
// StopContainer cancels the timer so normal shutdowns do not keep a goroutine alive.
func (r *BotRunner) trackDeadline(containerID string) {
	if r.cfg.MaxExecutionTime <= 0 {
		return
	}

	deadlineCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.deadlines[containerID] = cancel
	r.mu.Unlock()

	go func() {
		timer := time.NewTimer(r.cfg.MaxExecutionTime)
		defer timer.Stop()
		select {
		case <-deadlineCtx.Done():
			return
		case <-timer.C:
		}

		r.mu.Lock()
		_, active := r.deadlines[containerID]
		r.mu.Unlock()
		if !active {
			return
		}

		slog.Warn("Bot execution deadline exceeded, stopping container",
			"container_id", containerID, "deadline", r.cfg.MaxExecutionTime)
		if err := r.StopContainer(context.Background(), containerID); err != nil {
			slog.Error("Failed to stop container after deadline", "container_id", containerID, "error", err)
		}
	}()
}

func (r *BotRunner) cancelDeadline(containerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cancel, ok := r.deadlines[containerID]; ok {
		cancel()
		delete(r.deadlines, containerID)
	}
}

func (r *BotRunner) StopContainer(parent context.Context, containerID string) error {
	if strings.TrimSpace(containerID) == "" {
		return fmt.Errorf("container_id is required")
	}
	r.cancelDeadline(containerID)

	ctx, cancel := context.WithTimeout(parent, r.cfg.StopTimeout)
	defer cancel()

	grace := int(r.cfg.StopTimeout.Seconds())
	if err := r.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &grace}); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}
		slog.Warn("Failed to stop container gracefully, killing instead", "container_id", containerID, "error", err)
		if killErr := r.cli.ContainerKill(ctx, containerID, "SIGKILL"); killErr != nil && !client.IsErrNotFound(killErr) {
			slog.Error("Failed to kill container", "container_id", containerID, "error", killErr)
		}
	}

	// AutoRemove is best-effort; explicitly remove the container as well. Use a
	// fresh context so cancellation of the caller cannot skip cleanup.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), r.cfg.StopTimeout)
	defer cleanupCancel()
	if err := r.cli.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}

func (r *BotRunner) CountActiveContainers(parent context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(parent, r.cfg.StopTimeout)
	defer cancel()

	containers, err := r.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "managed_by=codearena-runtime")),
	})
	if err != nil {
		return 0, err
	}
	return len(containers), nil
}

func (r *BotRunner) removeIfExists(ctx context.Context, name string) error {
	err := r.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return err
	}
	return nil
}
