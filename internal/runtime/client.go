package runtime

import (
	"context"
	"fmt"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is the subset of RuntimeService used by the engine. Keeping it as an
// interface lets engine tests drive bot lifecycle without Docker.
type Client interface {
	StartBot(ctx context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error)
	StopBot(ctx context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error)
	Close() error
}

// GRPCClient is a lazy gRPC connection to the Runtime service. Calls are
// authenticated with the shared RUNTIME_AUTH_TOKEN; TLS termination is the
// responsibility of the deployment (same trust model as the other services).
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.RuntimeServiceClient
}

var _ Client = (*GRPCClient)(nil)

func NewGRPCClient(addr string) (*GRPCClient, error) {
	if addr == "" {
		return nil, fmt.Errorf("runtime address is empty")
	}
	authCfg, err := AuthConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("runtime client authentication: %w", err)
	}

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if authCfg.Token != "" {
		opts = append(opts, grpc.WithUnaryInterceptor(UnaryTokenClientInterceptor(authCfg.Token)))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("create runtime client for %s: %w", addr, err)
	}
	return &GRPCClient{conn: conn, client: pb.NewRuntimeServiceClient(conn)}, nil
}

func (c *GRPCClient) StartBot(ctx context.Context, req *pb.StartBotRequest) (*pb.StartBotResponse, error) {
	return c.client.StartBot(ctx, req)
}

func (c *GRPCClient) StopBot(ctx context.Context, req *pb.StopBotRequest) (*pb.StopBotResponse, error) {
	return c.client.StopBot(ctx, req)
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
