package clock

import (
	"testing"
)

func TestVectorClock_Increment(t *testing.T) {
	vc := New()
	vc.Increment("node1")
	if vc.Get("node1") != 1 {
		t.Errorf("Expected counter 1, got %d", vc.Get("node1"))
	}

	vc.Increment("node1")
	if vc.Get("node1") != 2 {
		t.Errorf("Expected counter 2, got %d", vc.Get("node1"))
	}

	vc.Increment("node2")
	if vc.Get("node2") != 1 {
		t.Errorf("Expected counter 1 for node2, got %d", vc.Get("node2"))
	}
}

func TestVectorClock_Merge(t *testing.T) {
	vc1 := New()
	vc1.Set("node1", 3)
	vc1.Set("node2", 1)

	vc2 := New()
	vc2.Set("node1", 2)
	vc2.Set("node2", 5)
	vc2.Set("node3", 1)

	vc1.Merge(vc2)

	if vc1.Get("node1") != 3 {
		t.Errorf("Expected 3 (max), got %d", vc1.Get("node1"))
	}
	if vc1.Get("node2") != 5 {
		t.Errorf("Expected 5 (max), got %d", vc1.Get("node2"))
	}
	if vc1.Get("node3") != 1 {
		t.Errorf("Expected 1, got %d", vc1.Get("node3"))
	}
}

func TestVectorClock_Compare(t *testing.T) {
	tests := []struct {
		name     string
		vc1      VectorClock
		vc2      VectorClock
		expected CompareResult
	}{
		{
			name:     "equal clocks",
			vc1:      VectorClock{"node1": 1, "node2": 2},
			vc2:      VectorClock{"node1": 1, "node2": 2},
			expected: Equal,
		},
		{
			name:     "vc1 before vc2",
			vc1:      VectorClock{"node1": 1, "node2": 1},
			vc2:      VectorClock{"node1": 2, "node2": 2},
			expected: Before,
		},
		{
			name:     "vc1 after vc2",
			vc1:      VectorClock{"node1": 2, "node2": 2},
			vc2:      VectorClock{"node1": 1, "node2": 1},
			expected: After,
		},
		{
			name:     "concurrent: vc1 has higher node1, vc2 has higher node2",
			vc1:      VectorClock{"node1": 2, "node2": 1},
			vc2:      VectorClock{"node1": 1, "node2": 2},
			expected: Concurrent,
		},
		{
			name:     "vc1 before vc2 (subset)",
			vc1:      VectorClock{"node1": 1},
			vc2:      VectorClock{"node1": 2, "node2": 1},
			expected: Before,
		},
		{
			name:     "concurrent (subset with different values)",
			vc1:      VectorClock{"node1": 2},
			vc2:      VectorClock{"node1": 1, "node2": 2},
			expected: Concurrent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.vc1.Compare(tt.vc2)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestVectorClock_Copy(t *testing.T) {
	vc1 := New()
	vc1.Set("node1", 5)
	vc1.Set("node2", 3)

	vc2 := vc1.Copy()
	if !vc1.Equal(vc2) {
		t.Error("Copy should be equal to original")
	}

	vc2.Increment("node1")
	if vc1.Get("node1") == vc2.Get("node1") {
		t.Error("Modifying copy should not affect original")
	}
}

func TestVectorClock_Dominates(t *testing.T) {
	vc1 := VectorClock{"node1": 2, "node2": 2}
	vc2 := VectorClock{"node1": 1, "node2": 1}

	if !vc1.Dominates(vc2) {
		t.Error("vc1 should dominate vc2")
	}

	if vc2.Dominates(vc1) {
		t.Error("vc2 should not dominate vc1")
	}
}

func TestVectorClock_IsConcurrent(t *testing.T) {
	vc1 := VectorClock{"node1": 2, "node2": 1}
	vc2 := VectorClock{"node1": 1, "node2": 2}

	if !vc1.IsConcurrent(vc2) {
		t.Error("vc1 and vc2 should be concurrent")
	}

	vc3 := VectorClock{"node1": 2, "node2": 2}
	if vc1.IsConcurrent(vc3) {
		t.Error("vc1 and vc3 should not be concurrent (vc3 dominates)")
	}
}
