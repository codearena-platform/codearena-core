package realtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/codearena-platform/codearena-core/internal/security"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 54 * time.Second
	maxMessageSize = 1024
)

type Hub struct {
	clients    map[string]map[*Client]bool // matchID -> clients
	broadcast  chan MatchUpdate            // Internal broadcast channel
	register   chan *Client
	unregister chan *Client
	redis      *redis.Client
	nodeID     string
	watching   map[string]bool // matchID -> isBeingWatched
	mu         sync.RWMutex
	done       chan struct{}
}

type MatchUpdate struct {
	MatchID string
	Data    []byte
}

type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	send    chan []byte
	matchID string
}

func newHub(rdb *redis.Client) *Hub {
	hostname, _ := os.Hostname()
	return &Hub{
		broadcast:  make(chan MatchUpdate, 1000),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[string]map[*Client]bool),
		redis:      rdb,
		nodeID:     fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano()),
		watching:   make(map[string]bool),
		done:       make(chan struct{}),
	}
}

func (h *Hub) run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			return

		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.matchID] == nil {
				h.clients[client.matchID] = make(map[*Client]bool)
			}
			h.clients[client.matchID][client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.matchID]; ok {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					close(client.send)
				}
			}
			h.mu.Unlock()

		case update := <-h.broadcast:
			h.mu.RLock()
			if clients, ok := h.clients[update.MatchID]; ok {
				for client := range clients {
					select {
					case client.send <- update.Data:
					default:
						close(client.send)
						delete(clients, client)
					}
				}
			}
			h.mu.RUnlock()
		}
	}
}

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 10 * time.Second,
	ReadBufferSize:   1024,
	WriteBufferSize:  1024,
	CheckOrigin: func(r *http.Request) bool {
		return security.OriginAllowed(r.Header.Get("Origin"), r, security.AllowedOriginsFromEnv())
	},
}

// validateToken keeps the package-level API used by existing callers while
// delegating to the shared, hardened JWT validation.
func validateToken(tokenString string) (*jwt.Token, error) {
	return security.ValidateToken(tokenString)
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request, matchID string) {
	// Authentication: Authorization: Bearer <token> only. Query strings are not
	// accepted because URLs are commonly logged by proxies and browsers.
	tokenString, err := security.TokenFromRequest(r)
	if err != nil {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	token, err := validateToken(tokenString)
	if err != nil || token == nil || !token.Valid {
		slog.Warn("Unauthorized access attempt", "match_id", matchID, "error", err)
		http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "match_id", matchID, "error", err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), matchID: matchID}
	select {
	case client.hub.register <- client:
	case <-client.hub.done:
		conn.Close()
		return
	}

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		select {
		case c.hub.unregister <- c:
		case <-c.hub.done:
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// gRPC Stream Consumer with Leader Election via Redis
func watchMatchConfig(ctx context.Context, client pb.MatchServiceClient, matchID string, hub *Hub) {
	lockKey := fmt.Sprintf("match:watcher:%s", matchID)
	defer func() {
		hub.mu.Lock()
		delete(hub.watching, matchID)
		hub.mu.Unlock()

		releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := hub.redis.Del(releaseCtx, lockKey).Err(); err != nil {
			slog.Warn("Failed to release watcher lock", "match_id", matchID, "error", err)
		}
	}()

	// Leader Election Loop
	for ctx.Err() == nil {
		// Try to acquire lock
		ok, err := hub.redis.SetNX(ctx, lockKey, hub.nodeID, 10*time.Second).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("Redis lock error", "match_id", matchID, "error", err)
			if !sleepCtx(ctx, 2*time.Second) {
				return
			}
			continue
		}

		if !ok {
			// Check if we are already the leader (heartbeat)
			val, getErr := hub.redis.Get(ctx, lockKey).Result()
			if getErr == nil && val == hub.nodeID {
				hub.redis.Expire(ctx, lockKey, 10*time.Second)
			} else {
				if !sleepCtx(ctx, 5*time.Second) {
					return
				}
				continue
			}
		}

		// We are the leader! Start watching gRPC if not already doing so
		slog.Info("Acting as watcher leader", "match_id", matchID, "node_id", hub.nodeID)
		stream, err := client.WatchMatch(ctx, &pb.MatchRequest{MatchId: matchID})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("Error watching match", "match_id", matchID, "error", err)
			if !sleepCtx(ctx, 2*time.Second) {
				return
			}
			continue
		}

		finished := func() bool {
			defer stream.CloseSend()
			for {
				state, err := stream.Recv()
				if err != nil {
					if err != io.EOF && ctx.Err() == nil {
						slog.Error("Stream error", "match_id", matchID, "error", err)
					}
					return false
				}

				// Publish to Redis
				jsonMsg := fmt.Sprintf(`{"match_id":"%s", "tick":%d, "status": "%s"}`, matchID, state.Tick, state.Status)
				if err := hub.redis.Publish(ctx, fmt.Sprintf("match:%s", matchID), jsonMsg).Err(); err != nil && ctx.Err() == nil {
					slog.Warn("Failed to publish match update", "match_id", matchID, "error", err)
				}

				// Refresh lock
				hub.redis.Expire(ctx, lockKey, 10*time.Second)

				if state.Status == pb.MatchStatus_FINISHED {
					slog.Info("Match finished, stopping watcher", "match_id", matchID)
					return true
				}
			}
		}()
		if finished || !sleepCtx(ctx, time.Second) {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *Hub) subscribeToRedis(ctx context.Context) {
	pubsub := h.redis.PSubscribe(ctx, "match:*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// match:ID -> Extract ID
			matchID := msg.Channel[len("match:"):]
			select {
			case h.broadcast <- MatchUpdate{MatchID: matchID, Data: []byte(msg.Payload)}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func newRealtimeHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// StartRealtime runs the realtime service until it receives a termination signal.
func StartRealtime(port string, competitionAddr string, redisAddr string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return StartRealtimeWithContext(ctx, port, competitionAddr, redisAddr)
}

// StartRealtimeWithContext runs the realtime service until ctx is canceled or
// one of its servers fails. All goroutines are tied to ctx so shutdown does not
// leak stream watchers, Redis subscriptions or WebSocket pumps.
func StartRealtimeWithContext(ctx context.Context, port string, competitionAddr string, redisAddr string) error {
	slog.Info("CodeArena Realtime Service starting", "port", port, "redis", redisAddr)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("Realtime could not connect to Redis", "address", redisAddr, "error", err)
		// We proceed, but horizontal scaling won't work correctly
	}

	hub := newHub(rdb)
	go hub.run(ctx)
	go hub.subscribeToRedis(ctx)

	var matchClient pb.MatchServiceClient
	conn, err := grpc.Dial(competitionAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Warn("Realtime could not connect to gRPC", "address", competitionAddr, "error", err)
	} else {
		defer conn.Close()
		matchClient = pb.NewMatchServiceClient(conn)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/match/", func(w http.ResponseWriter, r *http.Request) {
		matchID := r.URL.Path[len("/ws/match/"):]
		serveWs(hub, w, r, matchID)
		if matchClient != nil && matchID != "" {
			hub.mu.Lock()
			if !hub.watching[matchID] {
				hub.watching[matchID] = true
				go watchMatchConfig(ctx, matchClient, matchID, hub)
			}
			hub.mu.Unlock()
		}
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	srv := newRealtimeHTTPServer(port, mux)
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("realtime HTTP server failed: %w", err)
		}
	}()

	slog.Info("Realtime Service listening", "port", port)
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("realtime server shutdown failed: %w", err)
	}

	slog.Info("Server exiting")
	return nil
}
