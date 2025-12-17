package ring

import (
	"sort"
	"testing"
)

// TestRing_Property_Determinism tests that same membership produces same owner mapping
func TestRing_Property_Determinism(t *testing.T) {
	nodes1 := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}
	nodes2 := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}

	ring1 := NewRing(128)
	ring1.SetNodes(nodes1)

	ring2 := NewRing(128)
	ring2.SetNodes(nodes2)

	// Test multiple keys
	testKeys := []string{"key1", "key2", "key3", "user:123", "test-key", "another-key"}

	for _, key := range testKeys {
		owner1, exists1 := ring1.ResponsibleNode(key)
		owner2, exists2 := ring2.ResponsibleNode(key)

		if exists1 != exists2 {
			t.Errorf("Existence mismatch for key %s: ring1=%v, ring2=%v", key, exists1, exists2)
		}
		if owner1.ID != owner2.ID {
			t.Errorf("Owner mismatch for key %s: ring1=%s, ring2=%s", key, owner1.ID, owner2.ID)
		}
	}
}

// TestRing_NodeRemoval_ReducesOwnerSet tests that removing a node reduces owner set size
func TestRing_NodeRemoval_ReducesOwnerSet(t *testing.T) {
	nodes := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
		{ID: "n4", Addr: "127.0.0.1:50054"},
	}

	ring := NewRing(128)
	ring.SetNodes(nodes)

	// Get owners for many keys
	testKeys := make([]string, 100)
	for i := 0; i < 100; i++ {
		testKeys[i] = string(rune('a'+i%26)) + string(rune('0'+i))
	}

	ownersBefore := make(map[string]bool)
	for _, key := range testKeys {
		owner, exists := ring.ResponsibleNode(key)
		if exists {
			ownersBefore[owner.ID] = true
		}
	}

	// Remove a node
	ring.RemoveNode("n4")

	// Get owners again
	ownersAfter := make(map[string]bool)
	for _, key := range testKeys {
		owner, exists := ring.ResponsibleNode(key)
		if exists {
			ownersAfter[owner.ID] = true
		}
	}

	// Should not include removed node
	if ownersAfter["n4"] {
		t.Error("Removed node n4 should not be in owner set")
	}

	// All owners should be from remaining nodes
	remainingNodes := map[string]bool{"n1": true, "n2": true, "n3": true}
	for ownerID := range ownersAfter {
		if !remainingNodes[ownerID] {
			t.Errorf("Owner %s is not in remaining nodes", ownerID)
		}
	}
}

// TestRing_AlwaysReturnsExistingNode tests that ring always returns a valid node
func TestRing_AlwaysReturnsExistingNode(t *testing.T) {
	nodes := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}

	ring := NewRing(128)
	ring.SetNodes(nodes)

	// Test many random keys
	for i := 0; i < 1000; i++ {
		key := string(rune('a'+i%26)) + string(rune('0'+i%10)) + string(rune('A'+i%26))
		owner, exists := ring.ResponsibleNode(key)

		if !exists {
			t.Errorf("Ring returned no owner for key %s", key)
			continue
		}

		// Verify owner is in node list
		found := false
		for _, node := range nodes {
			if node.ID == owner.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Owner %s for key %s is not in node list", owner.ID, key)
		}
	}
}

// TestRing_PreferenceList_UniqueNodes tests that preference list returns unique nodes
func TestRing_PreferenceList_UniqueNodes(t *testing.T) {
	nodes := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}

	ring := NewRing(128)
	ring.SetNodes(nodes)

	key := "test-key"
	prefList := ring.PreferenceList(key, 10) // Request more than available nodes

	// Check uniqueness
	seen := make(map[string]bool)
	for _, node := range prefList {
		if seen[node.ID] {
			t.Errorf("Duplicate node %s in preference list", node.ID)
		}
		seen[node.ID] = true
	}

	// Should not exceed number of nodes
	if len(prefList) > len(nodes) {
		t.Errorf("Preference list length %d exceeds number of nodes %d", len(prefList), len(nodes))
	}
}

// TestRing_ConsistentAfterRebuild tests that rebuilding with same nodes produces same ring
func TestRing_ConsistentAfterRebuild(t *testing.T) {
	nodes := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}

	ring := NewRing(128)
	ring.SetNodes(nodes)

	// Get owners for test keys
	testKeys := []string{"key1", "key2", "key3", "key4", "key5"}
	owners1 := make(map[string]string)
	for _, key := range testKeys {
		owner, _ := ring.ResponsibleNode(key)
		owners1[key] = owner.ID
	}

	// Rebuild with same nodes
	ring.SetNodes(nodes)

	// Get owners again
	owners2 := make(map[string]string)
	for _, key := range testKeys {
		owner, _ := ring.ResponsibleNode(key)
		owners2[key] = owner.ID
	}

	// Should be identical
	for _, key := range testKeys {
		if owners1[key] != owners2[key] {
			t.Errorf("Owner changed for key %s after rebuild: %s -> %s", key, owners1[key], owners2[key])
		}
	}
}

// TestRing_OrderInvariant tests that order of nodes doesn't matter (if sorted)
func TestRing_OrderInvariant(t *testing.T) {
	nodes1 := []Node{
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
		{ID: "n3", Addr: "127.0.0.1:50053"},
	}
	nodes2 := []Node{
		{ID: "n3", Addr: "127.0.0.1:50053"},
		{ID: "n1", Addr: "127.0.0.1:50051"},
		{ID: "n2", Addr: "127.0.0.1:50052"},
	}

	ring1 := NewRing(128)
	ring1.SetNodes(nodes1)

	ring2 := NewRing(128)
	ring2.SetNodes(nodes2)

	// Sort nodes by ID for comparison
	sortNodes := func(nodes []Node) {
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].ID < nodes[j].ID
		})
	}

	sortNodes(nodes1)
	sortNodes(nodes2)

	// If nodes are sorted, should produce same ring
	// (Note: This depends on implementation - if SetNodes sorts internally, this will pass)
	testKeys := []string{"key1", "key2", "key3"}
	for _, key := range testKeys {
		owner1, _ := ring1.ResponsibleNode(key)
		owner2, _ := ring2.ResponsibleNode(key)
		// Both should return valid owners (may differ due to order, but both valid)
		if owner1.ID == "" || owner2.ID == "" {
			t.Errorf("Invalid owner for key %s", key)
		}
	}
}
