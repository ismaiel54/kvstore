package clock

import (
	"testing"
)

func TestVectorClock_Compare_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		vc1      VectorClock
		vc2      VectorClock
		expected CompareResult
	}{
		{
			name:     "empty clocks are equal",
			vc1:      New(),
			vc2:      New(),
			expected: Equal,
		},
		{
			name:     "empty before non-empty",
			vc1:      New(),
			vc2:      VectorClock{"node1": 1},
			expected: Before,
		},
		{
			name:     "non-empty after empty",
			vc1:      VectorClock{"node1": 1},
			vc2:      New(),
			expected: After,
		},
		{
			name:     "subset before superset",
			vc1:      VectorClock{"node1": 1},
			vc2:      VectorClock{"node1": 1, "node2": 1},
			expected: Before,
		},
		{
			name:     "superset after subset",
			vc1:      VectorClock{"node1": 1, "node2": 1},
			vc2:      VectorClock{"node1": 1},
			expected: After,
		},
		{
			name:     "concurrent: different nodes",
			vc1:      VectorClock{"node1": 2},
			vc2:      VectorClock{"node2": 2},
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

func TestVectorClock_Merge_Comprehensive(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 5)
	vc1.Set("n2", 3)

	vc2 := New()
	vc2.Set("n1", 2)
	vc2.Set("n2", 7)
	vc2.Set("n3", 1)

	vc1.Merge(vc2)

	if vc1.Get("n1") != 5 {
		t.Errorf("Expected max(5,2)=5, got %d", vc1.Get("n1"))
	}
	if vc1.Get("n2") != 7 {
		t.Errorf("Expected max(3,7)=7, got %d", vc1.Get("n2"))
	}
	if vc1.Get("n3") != 1 {
		t.Errorf("Expected 1, got %d", vc1.Get("n3"))
	}
}

func TestVectorClock_Increment_ZeroToOne(t *testing.T) {
	vc := New()
	if vc.Get("node1") != 0 {
		t.Errorf("Expected 0 for new node, got %d", vc.Get("node1"))
	}

	vc.Increment("node1")
	if vc.Get("node1") != 1 {
		t.Errorf("Expected 1 after increment, got %d", vc.Get("node1"))
	}
}

func TestVectorClock_String_Deterministic(t *testing.T) {
	vc := New()
	vc.Set("z", 3)
	vc.Set("a", 1)
	vc.Set("m", 2)

	// String should be sorted
	str := vc.String()
	expected := "{a:1, m:2, z:3}"
	if str != expected {
		t.Errorf("Expected %s, got %s", expected, str)
	}
}

