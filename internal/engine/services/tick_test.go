package services

import (
	"testing"

	"github.com/codearena-platform/codearena-core/internal/persistence"
	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
)

func TestEventLogType(t *testing.T) {
	tests := []struct {
		name string
		ev   *pb.SimulationEvent
		want string
	}{
		{name: "death", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_Death{}}, want: persistence.EventTypeDeath},
		{name: "finish", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_MatchFinished{}}, want: persistence.EventTypeMatchFinished},
		{name: "other", ev: &pb.SimulationEvent{Event: &pb.SimulationEvent_HitWall{}}, want: "hit_wall"},
		{name: "nil event", ev: nil, want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventLogType(tt.ev); got != tt.want {
				t.Fatalf("eventLogType() = %q, want %q", got, tt.want)
			}
		})
	}
}
