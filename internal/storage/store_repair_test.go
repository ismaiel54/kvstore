package storage

import (
	"testing"

	"kvstore/internal/clock"
)

func TestInMemoryStore_PutRepair(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Create a version
	vc1 := clock.New()
	vc1.Set("node1", 1)
	vc1.Set("node2", 1)

	// Store initial value
	store.Put("key1", []byte("value1"), vc1, false)

	// Repair with dominating version (should overwrite)
	vc2 := clock.New()
	vc2.Set("node1", 2)
	vc2.Set("node2", 1)

	err := store.PutRepair("key1", []byte("value2"), vc2, false)
	if err != nil {
		t.Errorf("PutRepair should succeed: %v", err)
	}

	// Verify value was overwritten
	vv := store.Get("key1")
	if vv == nil {
		t.Fatal("Expected value to exist")
	}
	if string(vv.Value) != "value2" {
		t.Errorf("Expected value2, got %s", string(vv.Value))
	}
	// Version should be exact (not incremented)
	if vv.Version.Get("node1") != 2 {
		t.Errorf("Expected version node1=2, got %d", vv.Version.Get("node1"))
	}
}

func TestInMemoryStore_PutRepair_RejectsOlderVersion(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Create a newer version
	vc1 := clock.New()
	vc1.Set("node1", 2)
	vc1.Set("node2", 1)

	store.Put("key1", []byte("value1"), vc1, false)

	// Try to repair with older version (should be rejected)
	vc2 := clock.New()
	vc2.Set("node1", 1)
	vc2.Set("node2", 1)

	err := store.PutRepair("key1", []byte("value2"), vc2, false)
	if err != nil {
		t.Errorf("PutRepair should silently skip (not error): %v", err)
	}

	// Verify original value unchanged
	vv := store.Get("key1")
	if string(vv.Value) != "value1" {
		t.Errorf("Expected value1 to remain, got %s", string(vv.Value))
	}
}

func TestInMemoryStore_PutRepair_Tombstone(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Store initial value
	vc1 := clock.New()
	vc1.Set("node1", 1)
	store.Put("key1", []byte("value1"), vc1, false)

	// Repair with tombstone
	vc2 := clock.New()
	vc2.Set("node1", 2)
	vc2.Set("node2", 1)

	err := store.PutRepair("key1", nil, vc2, true)
	if err != nil {
		t.Errorf("PutRepair tombstone should succeed: %v", err)
	}

	// Verify tombstone stored
	vv := store.Get("key1")
	if vv == nil {
		t.Fatal("Expected tombstone to exist")
	}
	if !vv.Deleted {
		t.Error("Expected deleted flag to be true")
	}
}

