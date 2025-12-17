package repair

import (
	"testing"

	"kvstore/internal/clock"
)

func TestReconcile_SingleWinner(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 2)
	vc1.Set("n2", 1)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 1)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: []byte("value2"), Version: vc2, Deleted: false},
	}

	result := Reconcile(values, []string{"r1", "r2"})

	if !result.IsResolved() {
		t.Errorf("Expected resolved (single winner), got %d winners", len(result.Winners))
	}
	if len(result.Winners) != 1 {
		t.Errorf("Expected 1 winner, got %d", len(result.Winners))
	}
	if string(result.Winners[0].Value) != "value1" {
		t.Errorf("Expected winner value 'value1', got '%s'", string(result.Winners[0].Value))
	}
	if len(result.Stale) != 1 {
		t.Errorf("Expected 1 stale version, got %d", len(result.Stale))
	}
	if _, ok := result.Stale["r2"]; !ok {
		t.Error("Expected r2 to be marked as stale")
	}
}

func TestReconcile_ConcurrentWrites(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 2)
	vc1.Set("n2", 1)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 2)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: []byte("value2"), Version: vc2, Deleted: false},
	}

	result := Reconcile(values, []string{"r1", "r2"})

	if !result.HasConflict() {
		t.Error("Expected conflict (concurrent writes)")
	}
	if len(result.Winners) != 2 {
		t.Errorf("Expected 2 winners (siblings), got %d", len(result.Winners))
	}
	if len(result.Stale) != 0 {
		t.Errorf("Expected no stale versions, got %d", len(result.Stale))
	}
}

func TestReconcile_TombstoneDominates(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 1)

	vc2 := clock.New()
	vc2.Set("n1", 2)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: nil, Version: vc2, Deleted: true}, // Tombstone
	}

	result := Reconcile(values, []string{"r1", "r2"})

	if !result.IsResolved() {
		t.Error("Expected resolved (tombstone dominates)")
	}
	if len(result.Winners) != 1 {
		t.Errorf("Expected 1 winner (tombstone), got %d", len(result.Winners))
	}
	if !result.Winners[0].Deleted {
		t.Error("Expected winner to be tombstone")
	}
	if len(result.Stale) != 1 {
		t.Errorf("Expected 1 stale version, got %d", len(result.Stale))
	}
}

func TestReconcile_TombstoneConcurrent(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 2)
	vc1.Set("n2", 1)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 2)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: nil, Version: vc2, Deleted: true}, // Tombstone
	}

	result := Reconcile(values, []string{"r1", "r2"})

	if !result.HasConflict() {
		t.Error("Expected conflict (tombstone concurrent with value)")
	}
	if len(result.Winners) != 2 {
		t.Errorf("Expected 2 winners (value + tombstone), got %d", len(result.Winners))
	}

	// Check that both value and tombstone are in winners
	hasValue := false
	hasTombstone := false
	for _, w := range result.Winners {
		if w.Deleted {
			hasTombstone = true
		} else {
			hasValue = true
		}
	}
	if !hasValue || !hasTombstone {
		t.Error("Expected both value and tombstone in winners")
	}
}

func TestReconcile_EqualVersions(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 1)
	vc1.Set("n2", 1)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 1)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: []byte("value2"), Version: vc2, Deleted: false},
	}

	result := Reconcile(values, []string{"r1", "r2"})

	// Equal versions should be deduplicated (only one winner)
	if len(result.Winners) != 1 {
		t.Errorf("Expected 1 winner (equal versions deduplicated), got %d", len(result.Winners))
	}
}

func TestReconcile_EmptyList(t *testing.T) {
	result := Reconcile([]VersionedValue{}, []string{})

	if !result.IsNotFound() {
		t.Error("Expected not found for empty list")
	}
	if len(result.Winners) != 0 {
		t.Errorf("Expected 0 winners, got %d", len(result.Winners))
	}
}

func TestReconcile_ThreeWayConflict(t *testing.T) {
	vc1 := clock.New()
	vc1.Set("n1", 2)
	vc1.Set("n2", 1)
	vc1.Set("n3", 1)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 2)
	vc2.Set("n3", 1)

	vc3 := clock.New()
	vc3.Set("n1", 1)
	vc3.Set("n2", 1)
	vc3.Set("n3", 2)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: []byte("value2"), Version: vc2, Deleted: false},
		{Value: []byte("value3"), Version: vc3, Deleted: false},
	}

	result := Reconcile(values, []string{"r1", "r2", "r3"})

	if !result.HasConflict() {
		t.Error("Expected conflict (three concurrent writes)")
	}
	if len(result.Winners) != 3 {
		t.Errorf("Expected 3 winners, got %d", len(result.Winners))
	}
}

func TestReconcile_MixedDominanceAndConcurrency(t *testing.T) {
	// vc1 dominates vc2
	vc1 := clock.New()
	vc1.Set("n1", 2)
	vc1.Set("n2", 2)

	vc2 := clock.New()
	vc2.Set("n1", 1)
	vc2.Set("n2", 1)

	// vc3 is concurrent with vc1
	vc3 := clock.New()
	vc3.Set("n1", 1)
	vc3.Set("n2", 3)

	values := []VersionedValue{
		{Value: []byte("value1"), Version: vc1, Deleted: false},
		{Value: []byte("value2"), Version: vc2, Deleted: false},
		{Value: []byte("value3"), Version: vc3, Deleted: false},
	}

	result := Reconcile(values, []string{"r1", "r2", "r3"})

	if !result.HasConflict() {
		t.Error("Expected conflict (vc1 and vc3 are concurrent)")
	}
	if len(result.Winners) != 2 {
		t.Errorf("Expected 2 winners (vc1 and vc3), got %d", len(result.Winners))
	}
	if len(result.Stale) != 1 {
		t.Errorf("Expected 1 stale version (vc2), got %d", len(result.Stale))
	}
}

