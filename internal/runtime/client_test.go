package runtime

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryTokenClientInterceptorAddsBearerToken(t *testing.T) {
	var got string
	interceptor := UnaryTokenClientInterceptor("secret-token")
	err := interceptor(context.Background(), "/test", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok {
				return nil
			}
			values := md.Get(authorizationHeader)
			if len(values) > 0 {
				got = values[0]
			}
			return nil
		})
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if got != "Bearer secret-token" {
		t.Errorf("authorization = %q, want %q", got, "Bearer secret-token")
	}
}

func TestNewGRPCClientRequiresAddress(t *testing.T) {
	t.Setenv("RUNTIME_ALLOW_INSECURE", "true")
	if _, err := NewGRPCClient(""); err == nil {
		t.Fatal("NewGRPCClient accepted an empty address")
	}
}

func TestNewGRPCClientUsesSharedSecret(t *testing.T) {
	t.Setenv("RUNTIME_AUTH_TOKEN", "shared-secret")
	t.Setenv("RUNTIME_ALLOW_INSECURE", "")
	client, err := NewGRPCClient("localhost:50053")
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
}

func TestNewGRPCClientFailsWithoutAuth(t *testing.T) {
	t.Setenv("RUNTIME_AUTH_TOKEN", "")
	t.Setenv("RUNTIME_ALLOW_INSECURE", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("CODEARENA_ENV", "")
	if _, err := NewGRPCClient("localhost:50053"); err == nil {
		t.Fatal("NewGRPCClient succeeded without RUNTIME_AUTH_TOKEN")
	}
}
