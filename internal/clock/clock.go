package clock

import (
	"fmt"
	"sort"
	"strings"
)

// VectorClock represents a vector clock as a map from node ID to counter.
// Thread-safe operations should be handled by the caller.
type VectorClock map[string]int64

// New creates a new empty vector clock.
func New() VectorClock {
	return make(VectorClock)
}

// Increment increments the counter for the given node ID.
// If the node ID doesn't exist, it's initialized to 1.
func (vc VectorClock) Increment(nodeID string) {
	vc[nodeID]++
}

// Get returns the counter value for the given node ID, or 0 if not present.
func (vc VectorClock) Get(nodeID string) int64 {
	return vc[nodeID]
}

// Set sets the counter for the given node ID.
func (vc VectorClock) Set(nodeID string, value int64) {
	vc[nodeID] = value
}

// Merge merges another vector clock into this one, taking the maximum
// counter value for each node ID.
func (vc VectorClock) Merge(other VectorClock) {
	for nodeID, counter := range other {
		if vc[nodeID] < counter {
			vc[nodeID] = counter
		}
	}
}

// Copy creates a deep copy of the vector clock.
func (vc VectorClock) Copy() VectorClock {
	copy := New()
	for k, v := range vc {
		copy[k] = v
	}
	return copy
}

// CompareResult represents the result of comparing two vector clocks.
type CompareResult int

const (
	// Before indicates this clock happened before the other.
	Before CompareResult = iota
	// After indicates this clock happened after the other.
	After
	// Concurrent indicates the clocks are concurrent (no causal relationship).
	Concurrent
	// Equal indicates the clocks are equal.
	Equal
)

// Compare compares two vector clocks and returns their relationship.
// Returns:
//   - Equal: if all counters are equal
//   - Before: if this clock happened before other (all counters <=, at least one <)
//   - After: if this clock happened after other (all counters >=, at least one >)
//   - Concurrent: if neither dominates (some counters are greater, some are less)
func (vc VectorClock) Compare(other VectorClock) CompareResult {
	if vc.Equal(other) {
		return Equal
	}

	allNodes := make(map[string]bool)
	for nodeID := range vc {
		allNodes[nodeID] = true
	}
	for nodeID := range other {
		allNodes[nodeID] = true
	}

	var thisLess, thisGreater bool
	for nodeID := range allNodes {
		thisVal := vc[nodeID]
		otherVal := other[nodeID]
		if thisVal < otherVal {
			thisLess = true
		} else if thisVal > otherVal {
			thisGreater = true
		}
	}

	if thisLess && !thisGreater {
		return Before
	}
	if thisGreater && !thisLess {
		return After
	}
	return Concurrent
}

// Equal checks if two vector clocks are equal.
func (vc VectorClock) Equal(other VectorClock) bool {
	if len(vc) != len(other) {
		return false
	}
	for nodeID, counter := range vc {
		if other[nodeID] != counter {
			return false
		}
	}
	return true
}

// String returns a string representation of the vector clock.
func (vc VectorClock) String() string {
	if len(vc) == 0 {
		return "{}"
	}

	// Sort for deterministic output
	keys := make([]string, 0, len(vc))
	for k := range vc {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, vc[k]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Dominates returns true if this clock dominates (happened after) the other.
func (vc VectorClock) Dominates(other VectorClock) bool {
	return vc.Compare(other) == After
}

// IsConcurrent returns true if this clock is concurrent with the other.
func (vc VectorClock) IsConcurrent(other VectorClock) bool {
	return vc.Compare(other) == Concurrent
}

