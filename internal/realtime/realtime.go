package realtime

import (
	"context"
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

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
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
	}
}

func (h *Hub) run() {
	for {
		select {
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
	CheckOrigin: func(r *http.Request) bool { return true },
}

// validateToken validates the JWT token from the query parameter
func validateToken(tokenString string) (*jwt.Token, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not set")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validating algorithm
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	return token, err
}

func serveWs(hub *Hub, w http.ResponseWriter, r *http.Request, matchID string) {
	// Authentication
	tokenString := r.URL.Query().Get("token")
	if tokenString == "" {
		http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
		return
	}

	token, err := validateToken(tokenString)
	if err != nil || !token.Valid {
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
	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (c *Client) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for message := range c.send {
		c.conn.WriteMessage(websocket.TextMessage, message)
	}
	c.conn.WriteMessage(websocket.CloseMessage, []byte{})
}

// gRPC Stream Consumer with Leader Election via Redis
func watchMatchConfig(client pb.MatchServiceClient, matchID string, hub *Hub) {
	ctx := context.Background()
	lockKey := fmt.Sprintf("match:watcher:%s", matchID)

	// Leader Election Loop
	for {
		// Try to acquire lock
		ok, err := hub.redis.SetNX(ctx, lockKey, hub.nodeID, 10*time.Second).Result()
		if err != nil {
			slog.Warn("Redis lock error", "match_id", matchID, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		if !ok {
			// Check if we are already the leader (heartbeat)
			val, _ := hub.redis.Get(ctx, lockKey).Result()
			if val == hub.nodeID {
				hub.redis.Expire(ctx, lockKey, 10*time.Second)
			} else {
				// Not the leader, wait and retry
				time.Sleep(5 * time.Second)
				continue
			}
		}

		// We are the leader! Start watching gRPC if not already doing so
		slog.Info("Acting as watcher leader", "match_id", matchID, "node_id", hub.nodeID)

		stream, err := client.WatchMatch(ctx, &pb.MatchRequest{MatchId: matchID})
		if err != nil {
			slog.Error("Error watching match", "match_id", matchID, "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Inner loop for receiving updates
		for {
			// Heartbeat the lock in the background or periodically
			// For simplicity, we'll just check it every few messages or rely on the outer loop
			state, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					slog.Error("Stream error", "match_id", matchID, "error", err)
				}
				break
			}

			// Publish to Redis
			jsonMsg := fmt.Sprintf(`{"match_id":"%s", "tick":%d, "status": "%s"}`, matchID, state.Tick, state.Status)
			hub.redis.Publish(ctx, fmt.Sprintf("match:%s", matchID), jsonMsg)

			// Refresh lock
			hub.redis.Expire(ctx, lockKey, 10*time.Second)

			if state.Status == pb.MatchStatus_FINISHED {
				slog.Info("Match finished, stopping watcher", "match_id", matchID)
				hub.mu.Lock()
				delete(hub.watching, matchID)
				hub.mu.Unlock()
				return
			}
		}

		slog.Warn("Watcher leader lost stream or match finished", "match_id", matchID)
		time.Sleep(1 * time.Second)
	}
}

func (h *Hub) subscribeToRedis() {
	pubsub := h.redis.PSubscribe(context.Background(), "match:*")
	defer pubsub.Close()

	ch := pubsub.Channel()
	for msg := range ch {
		// match:ID -> Extract ID
		matchID := msg.Channel[len("match:"):]
		h.broadcast <- MatchUpdate{MatchID: matchID, Data: []byte(msg.Payload)}
	}
}

func StartRealtime(port string, competitionAddr string, redisAddr string) error {
	slog.Info("CodeArena Realtime Service starting", "port", port, "redis", redisAddr)

	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Warn("Realtime could not connect to Redis", "address", redisAddr, "error", err)
		// We proceed, but horizontal scaling won't work correctly
	}

	hub := newHub(rdb)
	go hub.run()
	go hub.subscribeToRedis()

	conn, err := grpc.Dial(competitionAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Warn("Realtime could not connect to gRPC", "address", competitionAddr, "error", err)
	}
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	matchClient := pb.NewMatchServiceClient(conn)

	http.HandleFunc("/ws/match/", func(w http.ResponseWriter, r *http.Request) {
		matchID := r.URL.Path[len("/ws/match/"):]
		serveWs(hub, w, r, matchID)
		if matchClient != nil {
			hub.mu.Lock()
			if !hub.watching[matchID] {
				hub.watching[matchID] = true
				go watchMatchConfig(matchClient, matchID, hub)
			}
			hub.mu.Unlock()
		}
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	srv := &http.Server{
		Addr: port,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server listen failed", "error", err)
			os.Exit(1)
		}
	}()

	slog.Info("Realtime Service listening", "port", port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		return err
	}

	slog.Info("Server exiting")
	return nil
}
