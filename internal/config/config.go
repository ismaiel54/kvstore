package config

import (
	"fmt"
	"strings"

	"kvstore/internal/ring"
)

// Peer represents a peer node in the cluster.
type Peer struct {
	ID   string
	Addr string
}

// Config holds the node configuration.
type Config struct {
	NodeID     string
	ListenAddr string
	Peers      []Peer
	VNodes     int
}

// ParsePeers parses a comma-separated list of peers in the format:
// "id1=addr1,id2=addr2,id3=addr3"
func ParsePeers(peersStr string) ([]Peer, error) {
	if peersStr == "" {
		return []Peer{}, nil
	}

	parts := strings.Split(peersStr, ",")
	peers := make([]Peer, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid peer format: %s (expected id=addr)", part)
		}

		id := strings.TrimSpace(kv[0])
		addr := strings.TrimSpace(kv[1])

		if id == "" || addr == "" {
			return nil, fmt.Errorf("peer ID and address cannot be empty: %s", part)
		}

		peers = append(peers, Peer{
			ID:   id,
			Addr: addr,
		})
	}

	return peers, nil
}

// BuildRingNodes converts config peers + self into ring.Node slice.
// Includes self node in the list.
func (c *Config) BuildRingNodes() []ring.Node {
	nodes := make([]ring.Node, 0, len(c.Peers)+1)

	// Add self
	nodes = append(nodes, ring.Node{
		ID:   c.NodeID,
		Addr: c.ListenAddr,
	})

	// Add peers
	for _, peer := range c.Peers {
		// Skip self if it appears in peers list
		if peer.ID != c.NodeID {
			nodes = append(nodes, ring.Node{
				ID:   peer.ID,
				Addr: peer.Addr,
			})
		}
	}

	return nodes
}
