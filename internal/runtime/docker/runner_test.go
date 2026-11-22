package docker

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }
func (notFoundError) NotFound()     {}

type fakeDockerClient struct {
	mu sync.Mutex

	inspectErr  error
	createErrs  []error
	startErr    error
	stopErr     error
	createIndex int

	createConfigs []*container.Config
	hostConfigs   []*container.HostConfig
	removed       []string
	pulledImages  []string
	startCalls    int
	stopCalls     int
	killCalls     int
	removeCalls   int
}

func (f *fakeDockerClient) ImageInspectWithRaw(context.Context, string) (dockerimage.InspectResponse, []byte, error) {
	return dockerimage.InspectResponse{}, nil, f.inspectErr
}

func (f *fakeDockerClient) ImagePull(_ context.Context, ref string, _ dockerimage.PullOptions) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulledImages = append(f.pulledImages, ref)
	return io.NopCloser(strings.NewReader("{}")), nil
}

func (f *fakeDockerClient) ContainerCreate(_ context.Context, config *container.Config, hostConfig *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createConfigs = append(f.createConfigs, config)
	f.hostConfigs = append(f.hostConfigs, hostConfig)
	var err error
	if f.createIndex < len(f.createErrs) {
		err = f.createErrs[f.createIndex]
	}
	f.createIndex++
	if err != nil {
		return container.CreateResponse{}, err
	}
	return container.CreateResponse{ID: "container-123"}, nil
}

func (f *fakeDockerClient) ContainerStart(context.Context, string, container.StartOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCalls++
	return f.startErr
}

func (f *fakeDockerClient) ContainerStop(context.Context, string, container.StopOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopCalls++
	return f.stopErr
}

func (f *fakeDockerClient) ContainerKill(context.Context, string, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.killCalls++
	return nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, id string, options container.RemoveOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !options.Force {
		return errors.New("expected forced removal")
	}
	f.removed = append(f.removed, id)
	f.removeCalls++
	return nil
}

func (f *fakeDockerClient) ContainerList(context.Context, container.ListOptions) ([]container.Summary, error) {
	return []container.Summary{{ID: "container-123"}}, nil
}

func (f *fakeDockerClient) counts() (starts, stops, kills, removes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls, f.stopCalls, f.killCalls, f.removeCalls
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestStartContainerIsHardenedAndPullsMissingImage(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.MaxExecutionTime = 0 // no deadline goroutine in this test
	cli := &fakeDockerClient{inspectErr: notFoundError{}, startErr: errors.New("start failed")}
	runner := newBotRunner(cli, cfg)

	id, err := runner.StartContainer(context.Background(), "codearena/bot-go:latest", "bot-1", []string{"BOT_ID=bot-1"})
	if err == nil || id != "" {
		t.Fatalf("expected start failure, got id=%q err=%v", id, err)
	}
	if len(cli.pulledImages) != 1 || cli.pulledImages[0] != "codearena/bot-go:latest" {
		t.Errorf("expected missing image to be pulled, got %v", cli.pulledImages)
	}
	if len(cli.hostConfigs) != 1 {
		t.Fatalf("expected one container creation, got %d", len(cli.hostConfigs))
	}
	host := cli.hostConfigs[0]
	if host.NetworkMode != "none" || !host.ReadonlyRootfs || len(host.CapDrop) == 0 {
		t.Errorf("unhosted sandbox options: %+v", host)
	}
	if host.Resources.Memory != cfg.MemoryBytes || host.Resources.PidsLimit == nil {
		t.Errorf("missing resource limits: %+v", host.Resources)
	}
	if _, stops, _, removes := cli.counts(); stops != 0 || removes != 1 {
		t.Errorf("expected cleanup without stop after failed start: stops=%d removes=%d", stops, removes)
	}
	if len(cli.removed) != 1 || cli.removed[0] != "container-123" {
		t.Errorf("expected created container to be removed, got %v", cli.removed)
	}
}

func TestStartContainerCleansUpConflictingName(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.MaxExecutionTime = 0
	cli := &fakeDockerClient{createErrs: []error{errors.New("Conflict: container named bot-bot-1 already exists")}}
	runner := newBotRunner(cli, cfg)

	id, err := runner.StartContainer(context.Background(), "codearena/bot-go:latest", "bot-1", nil)
	if err != nil || id != "container-123" {
		t.Fatalf("expected retry to succeed, got id=%q err=%v", id, err)
	}
	if cli.createIndex != 2 {
		t.Errorf("expected two create attempts, got %d", cli.createIndex)
	}
	if len(cli.removed) != 1 || cli.removed[0] != "bot-bot-1" {
		t.Errorf("expected stale container to be removed, got %v", cli.removed)
	}
}

func TestStartContainerRejectsUntrustedInput(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.AllowedImages = []string{"codearena/bot-go@sha256:abc"}
	cfg.RequireDigest = true
	cli := &fakeDockerClient{}
	runner := newBotRunner(cli, cfg)

	if _, err := runner.StartContainer(context.Background(), "evil/image:latest", "bot-1", nil); err == nil {
		t.Error("expected unlisted image to be rejected")
	}
	if _, err := runner.StartContainer(context.Background(), "codearena/bot-go@sha256:abc", "../escape", nil); err == nil {
		t.Error("expected invalid bot ID to be rejected")
	}
	if cli.createIndex != 0 {
		t.Errorf("no container should be created for rejected input, got %d attempts", cli.createIndex)
	}
}

func TestStopContainerKillsAndRemoves(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.MaxExecutionTime = 0
	cli := &fakeDockerClient{stopErr: errors.New("stop failed")}
	runner := newBotRunner(cli, cfg)

	if err := runner.StopContainer(context.Background(), "container-123"); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
	starts, stops, kills, removes := cli.counts()
	if starts != 0 || stops != 1 || kills != 1 || removes != 1 {
		t.Errorf("unexpected calls: starts=%d stops=%d kills=%d removes=%d", starts, stops, kills, removes)
	}
	if err := runner.StopContainer(context.Background(), "container-123"); err != nil {
		t.Fatalf("unexpected second stop: %v", err)
	}
}

func TestExecutionDeadlineStopsAndRemovesContainer(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.MaxExecutionTime = 50 * time.Millisecond
	cli := &fakeDockerClient{}
	runner := newBotRunner(cli, cfg)

	if _, err := runner.StartContainer(context.Background(), "codearena/bot-go:latest", "bot-1", nil); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	waitFor(t, func() bool {
		_, stops, _, removes := cli.counts()
		return stops > 0 && removes > 0
	})
}

func TestStopCancelsExecutionDeadline(t *testing.T) {
	cfg := DefaultSandboxConfig()
	cfg.MaxExecutionTime = 80 * time.Millisecond
	cli := &fakeDockerClient{}
	runner := newBotRunner(cli, cfg)

	if _, err := runner.StartContainer(context.Background(), "codearena/bot-go:latest", "bot-1", nil); err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if err := runner.StopContainer(context.Background(), "container-123"); err != nil {
		t.Fatalf("unexpected stop error: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	_, stops, _, removes := cli.counts()
	if stops != 1 || removes != 1 {
		t.Errorf("deadline should not stop an already removed container: stops=%d removes=%d", stops, removes)
	}
}

func TestCountActiveContainers(t *testing.T) {
	cli := &fakeDockerClient{}
	runner := newBotRunner(cli, DefaultSandboxConfig())
	count, err := runner.CountActiveContainers(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("unexpected count=%d err=%v", count, err)
	}
}
