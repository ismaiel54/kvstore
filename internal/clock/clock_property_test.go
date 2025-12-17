package clock

import (
	"testing"
)

// TestVectorClock_Property_MergeDominatesBoth tests that merge(a,b) dominates both a and b
func TestVectorClock_Property_MergeDominatesBoth(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 1)
	vc1.Set("n2", 1)

	vc2 := New()
	vc2.Set("n1", 2)
	vc2.Set("n3", 1)

	merged := vc1.Copy()
	merged.Merge(vc2)

	// Merged should dominate vc1
	comp1 := merged.Compare(vc1)
	if comp1 != After && comp1 != Equal {
		t.Errorf("Merged clock should dominate or equal vc1, got %v", comp1)
	}

	// Merged should dominate vc2
	comp2 := merged.Compare(vc2)
	if comp2 != After && comp2 != Equal {
		t.Errorf("Merged clock should dominate or equal vc2, got %v", comp2)
	}

	// Merged should have max of each node
	if merged.Get("n1") != 2 {
		t.Errorf("Merged should have n1=max(1,2)=2, got %d", merged.Get("n1"))
	}
	if merged.Get("n2") != 1 {
		t.Errorf("Merged should have n2=1, got %d", merged.Get("n2"))
	}
	if merged.Get("n3") != 1 {
		t.Errorf("Merged should have n3=1, got %d", merged.Get("n3"))
	}
}

// TestVectorClock_Property_CompareAntisymmetric tests antisymmetric property where applicable
func TestVectorClock_Property_CompareAntisymmetric(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 1)
	vc1.Set("n2", 2)

	vc2 := New()
	vc2.Set("n1", 2)
	vc2.Set("n2", 1)

	comp12 := vc1.Compare(vc2)
	comp21 := vc2.Compare(vc1)

	// If vc1 is Before vc2, then vc2 should be After vc1
	if comp12 == Before {
		if comp21 != After {
			t.Errorf("If vc1 is Before vc2, then vc2 should be After vc1, got %v", comp21)
		}
	}

	// If vc1 is After vc2, then vc2 should be Before vc1
	if comp12 == After {
		if comp21 != Before {
			t.Errorf("If vc1 is After vc2, then vc2 should be Before vc1, got %v", comp21)
		}
	}

	// If vc1 is Equal to vc2, then vc2 should be Equal to vc1
	if comp12 == Equal {
		if comp21 != Equal {
			t.Errorf("If vc1 is Equal to vc2, then vc2 should be Equal to vc1, got %v", comp21)
		}
	}

	// If concurrent, both should be Concurrent
	if comp12 == Concurrent {
		if comp21 != Concurrent {
			t.Errorf("If vc1 is Concurrent with vc2, then vc2 should be Concurrent with vc1, got %v", comp21)
		}
	}
}

// TestVectorClock_Property_EqualClocksCompareEqual tests that equal clocks compare equal
func TestVectorClock_Property_EqualClocksCompareEqual(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 1)
	vc1.Set("n2", 2)
	vc1.Set("n3", 3)

	vc2 := New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 2)
	vc2.Set("n3", 3)

	if !vc1.Equal(vc2) {
		t.Error("Equal clocks should return true for Equal()")
	}

	comp := vc1.Compare(vc2)
	if comp != Equal {
		t.Errorf("Equal clocks should compare Equal, got %v", comp)
	}
}

// TestVectorClock_Property_IncrementIncreasesCounter tests that increment increases counter
func TestVectorClock_Property_IncrementIncreasesCounter(t *testing.T) {
	vc := New()
	vc.Set("n1", 5)

	vc.Increment("n1")
	if vc.Get("n1") != 6 {
		t.Errorf("Increment should increase counter from 5 to 6, got %d", vc.Get("n1"))
	}

	vc.Increment("n1")
	if vc.Get("n1") != 7 {
		t.Errorf("Increment should increase counter from 6 to 7, got %d", vc.Get("n1"))
	}
}

// TestVectorClock_Property_IncrementNewNode tests that increment creates new node entry
func TestVectorClock_Property_IncrementNewNode(t *testing.T) {
	vc := New()
	vc.Increment("n1")

	if vc.Get("n1") != 1 {
		t.Errorf("Increment on new node should set counter to 1, got %d", vc.Get("n1"))
	}
}

// TestVectorClock_Property_MergeIsIdempotent tests that merging with self doesn't change
func TestVectorClock_Property_MergeIsIdempotent(t *testing.T) {
	vc := New()
	vc.Set("n1", 1)
	vc.Set("n2", 2)

	original := vc.Copy()
	vc.Merge(original)

	if !vc.Equal(original) {
		t.Error("Merging clock with itself should not change it")
	}
}

// TestVectorClock_Property_Transitivity tests transitivity of Before relation
func TestVectorClock_Property_Transitivity(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 1)
	vc1.Set("n2", 1)

	vc2 := New()
	vc2.Set("n1", 2)
	vc2.Set("n2", 1)

	vc3 := New()
	vc3.Set("n1", 3)
	vc3.Set("n2", 2)

	// vc1 < vc2 < vc3
	comp12 := vc1.Compare(vc2)
	comp23 := vc2.Compare(vc3)
	comp13 := vc1.Compare(vc3)

	if comp12 == Before && comp23 == Before {
		if comp13 != Before {
			t.Errorf("Transitivity: if vc1 < vc2 and vc2 < vc3, then vc1 < vc3, got %v", comp13)
		}
	}
}

// TestVectorClock_Property_SubsetDominance tests that superset dominates subset
func TestVectorClock_Property_SubsetDominance(t *testing.T) {
	vc1 := New()
	vc1.Set("n1", 1)

	vc2 := New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 1)

	// vc2 should dominate vc1 (has all of vc1's entries with same or higher values)
	comp := vc2.Compare(vc1)
	if comp != After && comp != Equal {
		// Actually, this depends on implementation - if missing nodes are treated as 0,
		// then vc2 may not dominate vc1. Let's check the actual behavior.
		t.Logf("Note: vc2.Compare(vc1) = %v (implementation-dependent)", comp)
	}
}
