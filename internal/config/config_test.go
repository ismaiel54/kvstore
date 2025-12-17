package config

import (
	"testing"
)

func TestParsePeers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Peer
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  []Peer{},
		},
		{
			name:  "single peer",
			input: "n1=127.0.0.1:50051",
			want: []Peer{
				{ID: "n1", Addr: "127.0.0.1:50051"},
			},
		},
		{
			name:  "multiple peers",
			input: "n1=127.0.0.1:50051,n2=127.0.0.1:50052,n3=127.0.0.1:50053",
			want: []Peer{
				{ID: "n1", Addr: "127.0.0.1:50051"},
				{ID: "n2", Addr: "127.0.0.1:50052"},
				{ID: "n3", Addr: "127.0.0.1:50053"},
			},
		},
		{
			name:  "with spaces",
			input: "n1 = 127.0.0.1:50051 , n2 = 127.0.0.1:50052",
			want: []Peer{
				{ID: "n1", Addr: "127.0.0.1:50051"},
				{ID: "n2", Addr: "127.0.0.1:50052"},
			},
		},
		{
			name:    "invalid format - no equals",
			input:   "n1:127.0.0.1:50051",
			wantErr: true,
		},
		{
			name:    "invalid format - empty ID",
			input:   "=127.0.0.1:50051",
			wantErr: true,
		},
		{
			name:    "invalid format - empty addr",
			input:   "n1=",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePeers(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePeers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParsePeers() length = %d, want %d", len(got), len(tt.want))
					return
				}
				for i := range got {
					if got[i].ID != tt.want[i].ID || got[i].Addr != tt.want[i].Addr {
						t.Errorf("ParsePeers()[%d] = %v, want %v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestConfig_BuildRingNodes(t *testing.T) {
	cfg := &Config{
		NodeID:     "n1",
		ListenAddr: "127.0.0.1:50051",
		Peers: []Peer{
			{ID: "n2", Addr: "127.0.0.1:50052"},
			{ID: "n3", Addr: "127.0.0.1:50053"},
		},
	}

	nodes := cfg.BuildRingNodes()
	if len(nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(nodes))
	}

	// Check that self is included
	foundSelf := false
	for _, node := range nodes {
		if node.ID == "n1" && node.Addr == "127.0.0.1:50051" {
			foundSelf = true
		}
	}
	if !foundSelf {
		t.Error("Self node not found in ring nodes")
	}
}
