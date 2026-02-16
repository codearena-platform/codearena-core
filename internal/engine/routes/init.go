package routes

import (
	_ "embed"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/codearena-platform/codearena-core/internal/engine/services"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func StartAll(grpcAddr, webAddr string, e *services.SimulationEngine, tickRate time.Duration) {
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Printf("ERROR: Simulation gRPC listener failed on %s: %v", grpcAddr, err)
		return
	}

	srv := NewSimulationServer(e, tickRate)
	grpcSrv := grpc.NewServer()
	pb.RegisterBotServiceServer(grpcSrv, srv)
	pb.RegisterSimulationServiceServer(grpcSrv, srv)
	pb.RegisterMatchServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", srv.HandleDashboardWS)
		// Root handler removed to decouple FE from Core
		log.Printf("Web Dashboard Data (WS) available at ws://localhost%s/ws", webAddr)
		http.ListenAndServe(webAddr, mux)
	}()

	log.Printf("GRPC Server listening at %s", grpcAddr)
	grpcSrv.Serve(lis)
}
