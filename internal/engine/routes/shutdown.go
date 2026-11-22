package routes

// Shutdown stops the simulation loop and closes every stream channel owned by
// this server. It must run before the gRPC/HTTP servers are stopped and before
// the engine database is closed, so in-flight ticks and broadcasts have
// finished and handlers are released from their select loops.
func (s *SimulationServer) Shutdown() {
	s.mu.Lock()
	loop := s.loop
	s.loop = nil
	s.mu.Unlock()

	if loop != nil {
		loop.Stop()
		<-loop.Done()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for botID, ch := range s.botChannels {
		delete(s.botChannels, botID)
		close(ch)
	}
	for ch := range s.dashboardChannels {
		delete(s.dashboardChannels, ch)
		close(ch)
	}
}
