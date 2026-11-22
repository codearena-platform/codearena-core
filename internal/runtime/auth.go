package runtime

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// AuthTokenEnv is the shared secret required to call any Runtime gRPC method.
	AuthTokenEnv = "RUNTIME_AUTH_TOKEN"
	// AllowInsecureEnv is the explicit local-development opt-out. It is ignored in production.
	AllowInsecureEnv = "RUNTIME_ALLOW_INSECURE"

	authorizationHeader = "authorization"
	runtimeTokenHeader  = "x-runtime-token"
)

// AuthConfig controls authentication on the Runtime gRPC server.
type AuthConfig struct {
	Token    string
	Insecure bool
}

// AuthConfigFromEnv loads the Runtime authentication configuration.
// A token is always required unless RUNTIME_ALLOW_INSECURE=true is explicitly
// set for local development. The opt-out is refused in production.
func AuthConfigFromEnv() (AuthConfig, error) {
	token := strings.TrimSpace(os.Getenv(AuthTokenEnv))
	insecure := strings.EqualFold(strings.TrimSpace(os.Getenv(AllowInsecureEnv)), "true")

	if isProduction() && insecure {
		return AuthConfig{}, fmt.Errorf("%s=true is not allowed in production; set %s", AllowInsecureEnv, AuthTokenEnv)
	}

	if token == "" {
		if !insecure {
			return AuthConfig{}, fmt.Errorf("%s must be set (set %s=true only for local development)", AuthTokenEnv, AllowInsecureEnv)
		}
		return AuthConfig{Insecure: true}, nil
	}

	return AuthConfig{Token: token, Insecure: insecure}, nil
}

func isProduction() bool {
	for _, key := range []string{"CODEARENA_ENV", "APP_ENV"} {
		env := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if env == "production" || env == "prod" {
			return true
		}
	}
	return false
}

// ServerOptions installs token authentication for unary and streaming methods.
func (c AuthConfig) ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(UnaryTokenInterceptor(c.Token)),
		grpc.ChainStreamInterceptor(StreamTokenInterceptor(c.Token)),
	}
}

// UnaryTokenInterceptor rejects calls without the configured shared secret.
// An empty expected token is only possible through the explicit local-dev opt-out.
func UnaryTokenInterceptor(expected string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, expected); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamTokenInterceptor applies the same check to streaming RPCs.
func StreamTokenInterceptor(expected string) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(stream.Context(), expected); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func authorize(ctx context.Context, expected string) error {
	if expected == "" {
		return nil // Explicit RUNTIME_ALLOW_INSECURE=true local development only.
	}

	token, ok := tokenFromContext(ctx)
	if !ok || !tokenMatches(token, expected) {
		return status.Error(codes.Unauthenticated, "missing or invalid runtime token")
	}
	return nil
}

func tokenFromContext(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	for _, value := range md.Get(authorizationHeader) {
		if len(value) > 7 && strings.EqualFold(value[:7], "Bearer ") {
			return strings.TrimSpace(value[7:]), true
		}
	}
	if values := md.Get(runtimeTokenHeader); len(values) > 0 {
		return strings.TrimSpace(values[len(values)-1]), true
	}
	return "", false
}

func tokenMatches(got, expected string) bool {
	gotSum := sha256.Sum256([]byte(got))
	expectedSum := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(gotSum[:], expectedSum[:]) == 1
}
