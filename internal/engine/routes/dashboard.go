package routes

import (
	"net/http"
	"time"

	"github.com/codearena-platform/codearena-core/internal/security"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"github.com/gorilla/websocket"
)

const (
	dashboardWriteWait = 10 * time.Second
	dashboardPongWait  = 60 * time.Second
	dashboardPingEvery = 54 * time.Second
)

func (s *SimulationServer) HandleDashboardWS(w http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
		CheckOrigin:      security.CheckOrigin(security.AllowedOriginsFromEnv()),
	}
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	// Read control frames and detect disconnects; otherwise a silent client
	// would keep this handler (and its state channel) alive forever.
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		c.SetReadLimit(512)
		if err := c.SetReadDeadline(time.Now().Add(dashboardPongWait)); err != nil {
			return
		}
		c.SetPongHandler(func(string) error {
			return c.SetReadDeadline(time.Now().Add(dashboardPongWait))
		})
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ch := make(chan *pb.WorldState, 100)
	s.mu.Lock()
	s.dashboardChannels[ch] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.dashboardChannels[ch] {
			delete(s.dashboardChannels, ch)
			close(ch)
		}
		s.mu.Unlock()
	}()

	ping := time.NewTicker(dashboardPingEvery)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-readDone:
			return
		case st, ok := <-ch:
			if !ok {
				return
			}
			if err := c.SetWriteDeadline(time.Now().Add(dashboardWriteWait)); err != nil {
				return
			}
			if err := c.WriteJSON(st); err != nil {
				return
			}
		case <-ping.C:
			if err := c.SetWriteDeadline(time.Now().Add(dashboardWriteWait)); err != nil {
				return
			}
			if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
