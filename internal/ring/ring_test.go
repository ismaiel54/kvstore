package ring

import (
	"fmt"
	"testing"
)

func TestRing_ResponsibleNode(t *testing.T) {
	ring := NewRing(64)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
		{ID: "node3", Addr: "127.0.0.1:50053"},
	}
	ring.SetNodes(nodes)

	// Test that same key always maps to same node (determinism)
	key := "test-key-123"
	node1, found1 := ring.ResponsibleNode(key)
	if !found1 {
		t.Fatal("Expected to find a responsible node")
	}

	node2, found2 := ring.ResponsibleNode(key)
	if !found2 {
		t.Fatal("Expected to find a responsible node")
	}

	if node1.ID != node2.ID {
		t.Errorf("Determinism failed: same key mapped to different nodes: %s vs %s", node1.ID, node2.ID)
	}
}

func TestRing_Determinism(t *testing.T) {
	ring1 := NewRing(64)
	ring2 := NewRing(64)

	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
		{ID: "node3", Addr: "127.0.0.1:50053"},
	}

	ring1.SetNodes(nodes)
	ring2.SetNodes(nodes)

	// Test multiple keys
	testKeys := []string{"key1", "key2", "key3", "key4", "key5", "key100", "key999"}

	for _, key := range testKeys {
		node1, _ := ring1.ResponsibleNode(key)
		node2, _ := ring2.ResponsibleNode(key)
		if node1.ID != node2.ID {
			t.Errorf("Determinism failed for key %s: %s != %s", key, node1.ID, node2.ID)
		}
	}
}

func TestRing_Distribution(t *testing.T) {
	ring := NewRing(128)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
		{ID: "node3", Addr: "127.0.0.1:50053"},
	}
	ring.SetNodes(nodes)

	// Test distribution across many keys
	distribution := make(map[string]int)
	numKeys := 1000

	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key-%d", i)
		node, found := ring.ResponsibleNode(key)
		if !found {
			t.Fatalf("Expected to find node for key %s", key)
		}
		distribution[node.ID]++
	}

	// Check that all nodes got some keys
	if len(distribution) != 3 {
		t.Errorf("Expected 3 nodes to have keys, got %d", len(distribution))
	}

	// Check that no single node has >90% of keys (sanity check)
	for nodeID, count := range distribution {
		percentage := float64(count) / float64(numKeys) * 100
		if percentage > 90 {
			t.Errorf("Node %s has %.2f%% of keys (too high)", nodeID, percentage)
		}
		if count == 0 {
			t.Errorf("Node %s has no keys", nodeID)
		}
	}
}

func TestRing_NodeRemoval(t *testing.T) {
	ring := NewRing(64)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
		{ID: "node3", Addr: "127.0.0.1:50053"},
	}
	ring.SetNodes(nodes)

	// Record ownership before removal
	beforeRemoval := make(map[string]string)
	testKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	for _, key := range testKeys {
		node, _ := ring.ResponsibleNode(key)
		beforeRemoval[key] = node.ID
	}

	// Remove a node
	ring.RemoveNode("node2")

	// Check that ring still works
	for _, key := range testKeys {
		node, found := ring.ResponsibleNode(key)
		if !found {
			t.Errorf("Expected to find node for key %s after removal", key)
		}
		// Ownership may have changed, but should still be valid
		if node.ID == "node2" {
			t.Errorf("Key %s still mapped to removed node node2", key)
		}
	}

	// Verify node2 is gone
	allNodes := ring.GetNodes()
	nodeIDs := make(map[string]bool)
	for _, node := range allNodes {
		nodeIDs[node.ID] = true
	}
	if nodeIDs["node2"] {
		t.Error("node2 should be removed from ring")
	}
}

func TestRing_AddNode(t *testing.T) {
	ring := NewRing(64)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
	}
	ring.SetNodes(nodes)

	// Add a new node
	ring.AddNode(Node{ID: "node2", Addr: "127.0.0.1:50052"})

	allNodes := ring.GetNodes()
	if len(allNodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(allNodes))
	}

	nodeIDs := make(map[string]bool)
	for _, node := range allNodes {
		nodeIDs[node.ID] = true
	}
	if !nodeIDs["node1"] || !nodeIDs["node2"] {
		t.Error("Expected both node1 and node2 in ring")
	}
}

func TestRing_EmptyRing(t *testing.T) {
	ring := NewRing(64)
	node, found := ring.ResponsibleNode("any-key")
	if found {
		t.Error("Expected no node found for empty ring")
	}
	if node.ID != "" {
		t.Error("Expected empty node for empty ring")
	}
}

func TestRing_PreferenceList(t *testing.T) {
	ring := NewRing(64)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
		{ID: "node3", Addr: "127.0.0.1:50053"},
	}
	ring.SetNodes(nodes)

	key := "test-key"
	prefList := ring.PreferenceList(key, 3)

	if len(prefList) != 3 {
		t.Errorf("Expected preference list of length 3, got %d", len(prefList))
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, node := range prefList {
		if seen[node.ID] {
			t.Errorf("Duplicate node %s in preference list", node.ID)
		}
		seen[node.ID] = true
	}

	// First node should be the responsible node
	responsible, _ := ring.ResponsibleNode(key)
	if prefList[0].ID != responsible.ID {
		t.Errorf("First node in preference list should be responsible node: got %s, expected %s", prefList[0].ID, responsible.ID)
	}
}

func TestRing_PreferenceListPartial(t *testing.T) {
	ring := NewRing(64)
	nodes := []Node{
		{ID: "node1", Addr: "127.0.0.1:50051"},
		{ID: "node2", Addr: "127.0.0.1:50052"},
	}
	ring.SetNodes(nodes)

	// Request more nodes than available
	prefList := ring.PreferenceList("key", 5)
	if len(prefList) != 2 {
		t.Errorf("Expected preference list of length 2 (only 2 nodes), got %d", len(prefList))
	}
}
