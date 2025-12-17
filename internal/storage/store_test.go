package storage

import (
	"reflect"
	"testing"
	"time"

	"kvstore/internal/clock"
)

func TestInMemoryStore_GetPut(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Put a value
	version := store.Put("key1", []byte("value1"), nil, false)
	if version == nil {
		t.Fatal("Expected non-nil version")
	}

	// Get the value
	vv := store.Get("key1")
	if vv == nil {
		t.Fatal("Expected non-nil value")
	}
	if string(vv.Value) != "value1" {
		t.Errorf("Expected 'value1', got '%s'", string(vv.Value))
	}
	if vv.Version.Get("node1") != 1 {
		t.Errorf("Expected version counter 1, got %d", vv.Version.Get("node1"))
	}
}

func TestInMemoryStore_GetNotFound(t *testing.T) {
	store := NewInMemoryStore("node1")
	vv := store.Get("nonexistent")
	if vv != nil {
		t.Error("Expected nil for non-existent key")
	}
}

func TestInMemoryStore_PutWithVersion(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Put with initial version
	initialVersion := clock.New()
	initialVersion.Set("node2", 5)
	version1 := store.Put("key1", []byte("value1"), initialVersion, false)

	// Version should merge and increment
	if version1.Get("node2") != 5 {
		t.Errorf("Expected node2 counter 5, got %d", version1.Get("node2"))
	}
	if version1.Get("node1") != 1 {
		t.Errorf("Expected node1 counter 1, got %d", version1.Get("node1"))
	}

	// Put again with updated version
	updatedVersion := clock.New()
	updatedVersion.Set("node2", 7)
	version2 := store.Put("key1", []byte("value2"), updatedVersion, false)

	// Should merge both versions
	if version2.Get("node2") != 7 {
		t.Errorf("Expected node2 counter 7 (max), got %d", version2.Get("node2"))
	}
	if version2.Get("node1") != 2 {
		t.Errorf("Expected node1 counter 2, got %d", version2.Get("node1"))
	}
}

func TestInMemoryStore_Delete(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Put a value
	store.Put("key1", []byte("value1"), nil, false)

	// Delete it (should increment version from 1 to 2)
	version := store.Delete("key1", nil)
	if version.Get("node1") != 2 {
		t.Errorf("Expected version counter 2 after delete (was 1 after put), got %d", version.Get("node1"))
	}

	// Get should return tombstone (not nil, but deleted=true)
	vv := store.Get("key1")
	if vv == nil {
		t.Error("Expected tombstone after delete, got nil")
	}
	if !vv.IsTombstone() {
		t.Error("Expected tombstone after delete")
	}
}

func TestInMemoryStore_ConcurrentAccess(t *testing.T) {
	store := NewInMemoryStore("node1")

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			store.Put("key1", []byte("value"), nil, false)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have a value
	vv := store.Get("key1")
	if vv == nil {
		t.Fatal("Expected value after concurrent writes")
	}
	if string(vv.Value) != "value" {
		t.Errorf("Expected 'value', got '%s'", string(vv.Value))
	}
}

func TestVersionedValue_IsExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	future := now.Add(1 * time.Hour)

	vv1 := &VersionedValue{ExpiresAt: nil}
	if vv1.IsExpired() {
		t.Error("Nil expiration should not be expired")
	}

	vv2 := &VersionedValue{ExpiresAt: &past}
	if !vv2.IsExpired() {
		t.Error("Past expiration should be expired")
	}

	vv3 := &VersionedValue{ExpiresAt: &future}
	if vv3.IsExpired() {
		t.Error("Future expiration should not be expired")
	}
}

func TestInMemoryStore_GetReturnsCopy(t *testing.T) {
	store := NewInMemoryStore("node1")
	store.Put("key1", []byte("value1"), nil, false)

	vv1 := store.Get("key1")
	vv2 := store.Get("key1")

	// Modify the returned value
	vv1.Value[0] = 'X'

	// Second get should not be affected
	if reflect.DeepEqual(vv1.Value, vv2.Value) {
		t.Error("Get should return independent copies")
	}
}

