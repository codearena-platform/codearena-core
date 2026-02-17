package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type BotRunner struct {
	cli *client.Client
}

func NewBotRunner() (*BotRunner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &BotRunner{cli: cli}, nil
}

func (r *BotRunner) StartContainer(ctx context.Context, image string, botID string, envVars []string) (string, error) {
	// Pull image if not present (simplified)
	// In production, we might want to ensure we always have the latest, or use a specific tag policy.
	_, _, err := r.cli.ImageInspectWithRaw(ctx, image)
	if client.IsErrNotFound(err) {
		slog.Info("Pulling docker image", "image", image)
		out, err := r.cli.ImagePull(ctx, image, dockerimage.PullOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to pull image: %w", err)
		}
		defer out.Close()
		io.Copy(io.Discard, out) // Drain output
	}

	// Container Config
	config := &container.Config{
		Image: image,
		Env:   envVars,
		Labels: map[string]string{
			"codearena.bot.id": botID,
			"managed_by":       "codearena-runtime",
		},
	}

	// Host Config
	hostConfig := &container.HostConfig{
		AutoRemove:     true,
		ReadonlyRootfs: true, // Prevent bots from writing to the filesystem
		NetworkMode:    "bridge",
		Resources: container.Resources{
			NanoCPUs:  500000000,         // 0.5 CPU
			Memory:    512 * 1024 * 1024, // 512MB
			PidsLimit: &[]int64{64}[0],   // Prevent fork bombs
		},
	}

	// For Windows, "host" network driver works differently (or doesn't exist for isolation=hyperv).
	// Safest default for dev is often port mapping, but we are connecting OUT to the server.
	// If we use "host" network on Linux it works. On Windows, we might need special handling.
	// For now, let's assume default bridge and passing host IP via env var is the way to go,
	// but codearena usually requires the bot to connect TO the server.

	resp, err := r.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("bot-%s", botID))
	if err != nil {
		// Attempt to remove if name conflict?
		if strings.Contains(err.Error(), "Conflict") {
			// clean up old one
			_ = r.cli.ContainerRemove(ctx, fmt.Sprintf("bot-%s", botID), container.RemoveOptions{Force: true})
			// retry
			resp, err = r.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("bot-%s", botID))
			if err != nil {
				return "", fmt.Errorf("failed to create container (retry): %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to create container: %w", err)
		}
	}

	if err := r.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

func (r *BotRunner) StopContainer(ctx context.Context, containerID string) error {
	timeout := 5 // seconds
	stopOptions := container.StopOptions{Timeout: &timeout}
	// Warning: container.StopOptions might be different in older SDK versions?
	// Checking compatibility... usually it's just timeout int in older versions or struct in newer.
	// We'll stick to a simple Stop for now, or just Kill.

	// Docker SDK v25+ uses StopOptions. Older might use just timeout.
	// Let's use ContainerStop which usually takes a timeout pointer or struct depending on version.
	// Given we are generating a new project, we might need to `go get` the SDK.

	if err := r.cli.ContainerStop(ctx, containerID, stopOptions); err != nil {
		// If failed, try kill
		slog.Warn("Failed to stop container gracefully, killing instead", "container_id", containerID, "error", err)
		return r.cli.ContainerKill(ctx, containerID, "SIGKILL")
	}
	return nil
}

func (r *BotRunner) CountActiveContainers(ctx context.Context) (int, error) {
	containers, err := r.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", "managed_by=codearena-runtime")),
	})
	if err != nil {
		return 0, err
	}
	return len(containers), nil
}
