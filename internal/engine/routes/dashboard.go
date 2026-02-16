package routes

import (
	"net/http"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"github.com/gorilla/websocket"
)

func (s *SimulationServer) HandleDashboardWS(w http.ResponseWriter, r *http.Request) {
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()

	ch := make(chan *pb.WorldState, 100)
	s.mu.Lock()
	s.dashboardChannels[ch] = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.dashboardChannels, ch)
		s.mu.Unlock()
	}()

	for st := range ch {
		if err := c.WriteJSON(st); err != nil {
			return
		}
	}
}
