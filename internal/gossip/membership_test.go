package gossip

import (
	"testing"
	"time"

	"kvstore/internal/ring"
)

func TestMembership_MergeRules(t *testing.T) {
	m := NewMembership("local", "127.0.0.1:50051", 1*time.Second, 3*time.Second, 10*time.Second)

	// Test: higher incarnation wins
	remote1 := []*Member{
		{ID: "node1", Addr: "127.0.0.1:50052", Status: Alive, Incarnation: 5},
	}
	m.ApplyGossip(remote1)

	member := m.members["node1"]
	if member == nil {
		t.Fatal("Expected node1 to be added")
	}
	if member.Incarnation != 5 {
		t.Errorf("Expected incarnation 5, got %d", member.Incarnation)
	}

	// Test: lower incarnation ignored
	remote2 := []*Member{
		{ID: "node1", Addr: "127.0.0.1:50052", Status: Suspect, Incarnation: 3},
	}
	m.ApplyGossip(remote2)

	if member.Incarnation != 5 {
		t.Errorf("Expected incarnation to remain 5, got %d", member.Incarnation)
	}
	if member.Status != Alive {
		t.Errorf("Expected status to remain Alive, got %v", member.Status)
	}

	// Test: same incarnation, prefer Alive
	remote3 := []*Member{
		{ID: "node1", Addr: "127.0.0.1:50052", Status: Alive, Incarnation: 5},
	}
	m.members["node1"].Status = Suspect
	m.ApplyGossip(remote3)

	if member.Status != Alive {
		t.Errorf("Expected status to update to Alive, got %v", member.Status)
	}
}

func TestMembership_AliveNodes(t *testing.T) {
	m := NewMembership("local", "127.0.0.1:50051", 1*time.Second, 3*time.Second, 10*time.Second)

	m.ApplyGossip([]*Member{
		{ID: "node1", Addr: "127.0.0.1:50052", Status: Alive, Incarnation: 1},
		{ID: "node2", Addr: "127.0.0.1:50053", Status: Suspect, Incarnation: 1},
		{ID: "node3", Addr: "127.0.0.1:50054", Status: Dead, Incarnation: 1},
	})

	alive := m.AliveNodes()
	if len(alive) != 2 { // local + node1
		t.Errorf("Expected 2 alive nodes, got %d", len(alive))
	}

	// Check that node1 is included
	found := false
	for _, node := range alive {
		if node.ID == "node1" {
			found = true
		}
	}
	if !found {
		t.Error("Expected node1 to be in alive nodes")
	}

	// Check that node2 (Suspect) is not included
	for _, node := range alive {
		if node.ID == "node2" {
			t.Error("Suspect node should not be in alive nodes")
		}
	}
}

func TestMembership_StateTransitions(t *testing.T) {
	m := NewMembership("local", "127.0.0.1:50051", 1*time.Second, 100*time.Millisecond, 200*time.Millisecond)

	m.ApplyGossip([]*Member{
		{ID: "node1", Addr: "127.0.0.1:50052", Status: Alive, Incarnation: 1},
	})

	// Mark as suspect
	m.mu.Lock()
	m.members["node1"].Status = Suspect
	m.members["node1"].LastSeen = time.Now().Add(-150 * time.Millisecond) // Past suspect timeout
	m.mu.Unlock()

	// Check timeouts
	m.checkTimeouts()

	m.mu.RLock()
	member := m.members["node1"]
	m.mu.RUnlock()

	if member.Status != Dead {
		t.Errorf("Expected node1 to be Dead after timeout, got %v", member.Status)
	}
}

func TestMembership_AddSeedMembers(t *testing.T) {
	m := NewMembership("local", "127.0.0.1:50051", 1*time.Second, 3*time.Second, 10*time.Second)

	seeds := []ring.Node{
		{ID: "seed1", Addr: "127.0.0.1:50052"},
		{ID: "seed2", Addr: "127.0.0.1:50053"},
	}

	m.AddSeedMembers(seeds)

	// Check that seeds are added
	m.mu.RLock()
	memberCount := len(m.members)
	seed1Exists := m.members["seed1"] != nil
	seed2Exists := m.members["seed2"] != nil
	m.mu.RUnlock()

	if memberCount != 3 { // local + 2 seeds
		t.Errorf("Expected 3 members, got %d", memberCount)
	}

	if !seed1Exists {
		t.Error("Expected seed1 to be added")
	}
	if !seed2Exists {
		t.Error("Expected seed2 to be added")
	}
}

