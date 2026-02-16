package main

import (
	"testing"
)

func TestDetermineRuntimeMode(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		expectLocal bool
		expectAddr  string
	}{
		{
			name:        "Empty address (Monolith)",
			addr:        "",
			expectLocal: true,
			expectAddr:  "localhost:50053",
		},
		{
			name:        "Localhost address (Monolith)",
			addr:        "localhost:50053",
			expectLocal: true,
			expectAddr:  "localhost:50053",
		},
		{
			name:        "Remote address (Hybrid)",
			addr:        "192.168.1.50:50053",
			expectLocal: false,
			expectAddr:  "192.168.1.50:50053",
		},
		{
			name:        "Standard Port address (Hybrid)",
			addr:        "engine.codearena.com:50053",
			expectLocal: false,
			expectAddr:  "engine.codearena.com:50053",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, addr := determineRuntimeMode(tt.addr)
			if local != tt.expectLocal {
				t.Errorf("expected local=%v, got %v", tt.expectLocal, local)
			}
			if addr != tt.expectAddr {
				t.Errorf("expected addr=%q, got %q", tt.expectAddr, addr)
			}
		})
	}
}
