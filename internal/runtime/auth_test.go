package runtime

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testRuntimeToken = "top-secret-token"

func TestAuthConfigFromEnv(t *testing.T) {
	t.Run("token required by default", func(t *testing.T) {
		t.Setenv(AuthTokenEnv, "")
		t.Setenv(AllowInsecureEnv, "")
		t.Setenv("CODEARENA_ENV", "")
		t.Setenv("APP_ENV", "")

		if _, err := AuthConfigFromEnv(); err == nil {
			t.Fatal("expected error when no token and no opt-out are configured")
		}
	})

	t.Run("token configured", func(t *testing.T) {
		t.Setenv(AuthTokenEnv, testRuntimeToken)
		t.Setenv(AllowInsecureEnv, "")
		t.Setenv("CODEARENA_ENV", "")

		cfg, err := AuthConfigFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Token != testRuntimeToken || cfg.Insecure {
			t.Fatalf("unexpected config: %+v", cfg)
		}
		if len(cfg.ServerOptions()) != 2 {
			t.Fatal("expected unary and stream interceptors")
		}
	})

	t.Run("local dev opt-out", func(t *testing.T) {
		t.Setenv(AuthTokenEnv, "")
		t.Setenv(AllowInsecureEnv, "true")
		t.Setenv("CODEARENA_ENV", "")

		cfg, err := AuthConfigFromEnv()
		if err != nil || !cfg.Insecure || cfg.Token != "" {
			t.Fatalf("expected insecure dev config, got %+v, err=%v", cfg, err)
		}
	})

	t.Run("opt-out refused in production", func(t *testing.T) {
		t.Setenv(AuthTokenEnv, "")
		t.Setenv(AllowInsecureEnv, "true")
		t.Setenv("CODEARENA_ENV", "production")

		if _, err := AuthConfigFromEnv(); err == nil {
			t.Fatal("expected production to refuse the insecure opt-out")
		}
	})

	t.Run("token allowed in production", func(t *testing.T) {
		t.Setenv(AuthTokenEnv, testRuntimeToken)
		t.Setenv(AllowInsecureEnv, "")
		t.Setenv("APP_ENV", "prod")

		if _, err := AuthConfigFromEnv(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestUnaryTokenInterceptor(t *testing.T) {
	interceptor := UnaryTokenInterceptor(testRuntimeToken)
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	tests := []struct {
		name    string
		md      metadata.MD
		wantErr bool
	}{
		{"bearer token", metadata.Pairs("authorization", "Bearer "+testRuntimeToken), false},
		{"dedicated header", metadata.Pairs("x-runtime-token", testRuntimeToken), false},
		{"missing token", metadata.MD{}, true},
		{"wrong token", metadata.Pairs("authorization", "Bearer wrong"), true},
		{"malformed authorization", metadata.Pairs("authorization", testRuntimeToken), true},
		{"non bearer scheme", metadata.Pairs("authorization", "Basic "+testRuntimeToken), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), tt.md)
			resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/codearena.v1.RuntimeService/StartBot"}, handler)
			if tt.wantErr {
				if status.Code(err) != codes.Unauthenticated {
					t.Fatalf("expected Unauthenticated, got %v", err)
				}
				return
			}
			if err != nil || resp != "ok" {
				t.Fatalf("expected authorized call, got resp=%v err=%v", resp, err)
			}
		})
	}
}

func TestStreamTokenInterceptor(t *testing.T) {
	interceptor := StreamTokenInterceptor(testRuntimeToken)
	handler := func(any, grpc.ServerStream) error { return nil }
	stream := &fakeServerStream{ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{})}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated for stream without token, got %v", err)
	}

	stream.ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+testRuntimeToken))
	if err := interceptor(nil, stream, &grpc.StreamServerInfo{}, handler); err != nil {
		t.Fatalf("expected authorized stream, got %v", err)
	}
}

func TestInsecureInterceptorAllowsCalls(t *testing.T) {
	interceptor := UnaryTokenInterceptor("")
	called := false
	_, err := interceptor(context.Background(), nil, nil, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if err != nil || !called {
		t.Fatalf("expected local-dev interceptor to allow the call, called=%v err=%v", called, err)
	}
}

func TestTokenMatchesIsNotLengthDependent(t *testing.T) {
	if !tokenMatches(testRuntimeToken, testRuntimeToken) {
		t.Fatal("expected matching tokens")
	}
	if tokenMatches(testRuntimeToken, testRuntimeToken+"x") {
		t.Fatal("expected different tokens to be rejected")
	}
}

func TestAllRuntimeMethodsRequireToken(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	cfg := AuthConfig{Token: testRuntimeToken}
	server := grpc.NewServer(cfg.ServerOptions()...)
	pb.RegisterRuntimeServiceServer(server, &authTestServer{})
	go server.Serve(lis)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		t.Fatalf("failed to dial test server: %v", err)
	}
	defer conn.Close()
	client := pb.NewRuntimeServiceClient(conn)

	withToken := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testRuntimeToken)
	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{"StartBot", func(ctx context.Context) error {
			_, err := client.StartBot(ctx, &pb.StartBotRequest{BotId: "bot-1", Image: "image"})
			return err
		}},
		{"StopBot", func(ctx context.Context) error {
			_, err := client.StopBot(ctx, &pb.StopBotRequest{ContainerId: "container-1"})
			return err
		}},
		{"GetRuntimeStats", func(ctx context.Context) error {
			_, err := client.GetRuntimeStats(ctx, &pb.Empty{})
			return err
		}},
	}
	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(ctx); status.Code(err) != codes.Unauthenticated {
				t.Fatalf("expected Unauthenticated without token, got %v", err)
			}
			if err := tt.call(withToken); err != nil {
				t.Fatalf("expected call with token to succeed, got %v", err)
			}
		})
	}
}

type authTestServer struct {
	pb.UnimplementedRuntimeServiceServer
}

func (*authTestServer) StartBot(context.Context, *pb.StartBotRequest) (*pb.StartBotResponse, error) {
	return &pb.StartBotResponse{Success: true}, nil
}

func (*authTestServer) StopBot(context.Context, *pb.StopBotRequest) (*pb.StopBotResponse, error) {
	return &pb.StopBotResponse{Success: true}, nil
}

func (*authTestServer) GetRuntimeStats(context.Context, *pb.Empty) (*pb.RuntimeStats, error) {
	return &pb.RuntimeStats{}, nil
}

type fakeServerStream struct {
	ctx context.Context
}

func (s *fakeServerStream) SetHeader(metadata.MD) error  { return errors.New("not implemented") }
func (s *fakeServerStream) SendHeader(metadata.MD) error { return errors.New("not implemented") }
func (s *fakeServerStream) SetTrailer(metadata.MD)       {}
func (s *fakeServerStream) Context() context.Context     { return s.ctx }
func (s *fakeServerStream) SendMsg(any) error            { return errors.New("not implemented") }
func (s *fakeServerStream) RecvMsg(any) error            { return errors.New("not implemented") }
