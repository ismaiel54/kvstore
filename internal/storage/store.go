package storage

import (
	"fmt"
	"sync"
	"time"

	"kvstore/internal/clock"
)

// VersionedValue represents a value with its vector clock version.
type VersionedValue struct {
	Value     []byte
	Version   clock.VectorClock
	Deleted   bool       // True if this is a tombstone (deleted)
	ExpiresAt *time.Time // nil if no expiration
}

// IsExpired checks if the value has expired.
func (vv *VersionedValue) IsExpired() bool {
	if vv.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*vv.ExpiresAt)
}

// IsTombstone checks if this is a deletion tombstone.
func (vv *VersionedValue) IsTombstone() bool {
	return vv.Deleted
}

// Store defines the interface for key-value storage.
type Store interface {
	// Get retrieves a value by key. Returns nil if not found or expired.
	Get(key string) *VersionedValue
	// Put stores a value with the given version. If version is nil, creates a new one.
	// If deleted is true, stores a tombstone.
	Put(key string, value []byte, version clock.VectorClock, deleted bool) clock.VectorClock
	// PutRepair stores a value with the exact version (no increment) for read repair.
	// Only overwrites if incoming version dominates or is equal to existing.
	PutRepair(key string, value []byte, version clock.VectorClock, deleted bool) error
	// Delete removes a key. Returns the version after deletion.
	Delete(key string, version clock.VectorClock) clock.VectorClock
}

// InMemoryStore is an in-memory implementation of Store.
// It's thread-safe and supports TTL expiration.
type InMemoryStore struct {
	mu     sync.RWMutex
	data   map[string]*VersionedValue
	nodeID string // Node ID for generating vector clocks
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore(nodeID string) *InMemoryStore {
	return &InMemoryStore{
		data:   make(map[string]*VersionedValue),
		nodeID: nodeID,
	}
}

// Get retrieves a value by key.
func (s *InMemoryStore) Get(key string) *VersionedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vv, exists := s.data[key]
	if !exists {
		return nil
	}

	if vv.IsExpired() {
		// Clean up expired entry (best effort, don't block readers)
		go s.deleteExpired(key)
		return nil
	}

	// Return a copy to avoid external modifications
	return &VersionedValue{
		Value:     append([]byte(nil), vv.Value...),
		Version:   vv.Version.Copy(),
		Deleted:   vv.Deleted,
		ExpiresAt: copyTime(vv.ExpiresAt),
	}
}

// Put stores a value with the given version.
// If version is nil, creates a new vector clock and increments it.
// Otherwise, merges the provided version and increments.
// If deleted is true, stores a tombstone.
func (s *InMemoryStore) Put(key string, value []byte, version clock.VectorClock, deleted bool) clock.VectorClock {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newVersion clock.VectorClock
	if version == nil {
		newVersion = clock.New()
	} else {
		newVersion = version.Copy()
	}

	// Merge with existing version if present
	if existing, exists := s.data[key]; exists && !existing.IsExpired() {
		newVersion.Merge(existing.Version)
	}

	// Increment for this node
	newVersion.Increment(s.nodeID)

	// Store the value (or tombstone)
	var valueCopy []byte
	if !deleted {
		valueCopy = append([]byte(nil), value...)
	}
	s.data[key] = &VersionedValue{
		Value:     valueCopy,
		Version:   newVersion,
		Deleted:   deleted,
		ExpiresAt: nil, // TTL will be handled in Phase 2+ if needed
	}

	return newVersion.Copy()
}

// PutRepair stores a value with the exact version (no increment) for read repair.
// Only overwrites if incoming version dominates or is equal to existing.
func (s *InMemoryStore) PutRepair(key string, value []byte, version clock.VectorClock, deleted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if version == nil {
		return fmt.Errorf("repair requires non-nil version")
	}

	// Check if we should overwrite
	if existing, exists := s.data[key]; exists && !existing.IsExpired() {
		comp := version.Compare(existing.Version)
		// Only overwrite if incoming dominates or is equal
		if comp != clock.After && comp != clock.Equal {
			// Incoming version is before or concurrent - don't overwrite
			return nil // Silently skip (best effort)
		}
	}

	// Overwrite with exact version (no increment)
	var valueCopy []byte
	if !deleted {
		valueCopy = append([]byte(nil), value...)
	}
	s.data[key] = &VersionedValue{
		Value:     valueCopy,
		Version:   version.Copy(), // Store exact version
		Deleted:   deleted,
		ExpiresAt: nil,
	}

	return nil
}

// Delete removes a key.
func (s *InMemoryStore) Delete(key string, version clock.VectorClock) clock.VectorClock {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newVersion clock.VectorClock
	if version == nil {
		newVersion = clock.New()
	} else {
		newVersion = version.Copy()
	}

	// Merge with existing version if present
	if existing, exists := s.data[key]; exists && !existing.IsExpired() {
		newVersion.Merge(existing.Version)
	}

	// Increment for this node
	newVersion.Increment(s.nodeID)

	// Store tombstone instead of deleting (for replication)
	s.data[key] = &VersionedValue{
		Value:     nil,
		Version:   newVersion,
		Deleted:   true,
		ExpiresAt: nil,
	}

	return newVersion.Copy()
}

// deleteExpired removes an expired key (called asynchronously).
func (s *InMemoryStore) deleteExpired(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if vv, exists := s.data[key]; exists && vv.IsExpired() {
		delete(s.data, key)
	}
}

// copyTime creates a copy of a time pointer.
func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	copy := *t
	return &copy
}
