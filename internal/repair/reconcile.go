package repair

import (
	"kvstore/internal/clock"
)

// VersionedValue represents a value with its vector clock version.
// This is used for reconciliation and is compatible with storage.VersionedValue.
type VersionedValue struct {
	Value   []byte
	Version clock.VectorClock
	Deleted bool
}

// ReconcileResult represents the result of reconciling multiple versions.
type ReconcileResult struct {
	// Winners is the maximal set of non-dominated versions (siblings).
	// If len(Winners) == 1, there's a single winner.
	// If len(Winners) > 1, there are concurrent versions (conflicts).
	Winners []VersionedValue

	// Stale maps replica identifier to the stale version it returned.
	// A version is stale if it's dominated by at least one winner.
	Stale map[string]VersionedValue
}

// Reconcile computes the maximal set of versions from the given list.
// It returns winners (non-dominated versions) and stale versions (dominated ones).
// replicaIDs should correspond 1:1 with values (for tracking which replica returned which version).
func Reconcile(values []VersionedValue, replicaIDs []string) ReconcileResult {
	if len(values) == 0 {
		return ReconcileResult{
			Winners: []VersionedValue{},
			Stale:   make(map[string]VersionedValue),
		}
	}

	if len(replicaIDs) != len(values) {
		// If replica IDs don't match, create placeholder IDs
		replicaIDs = make([]string, len(values))
		for i := range replicaIDs {
			replicaIDs[i] = "replica-" + string(rune(i))
		}
	}

	// Build list of all versions with their replica IDs
	type versionWithReplica struct {
		value     VersionedValue
		replicaID string
		index     int
	}

	allVersions := make([]versionWithReplica, len(values))
	for i, v := range values {
		allVersions[i] = versionWithReplica{
			value:     v,
			replicaID: replicaIDs[i],
			index:     i,
		}
	}

	// Compute maximal set: discard any version dominated by another
	winners := make([]VersionedValue, 0)
	stale := make(map[string]VersionedValue)

	for i, v1 := range allVersions {
		isDominated := false

		// Check if v1 is dominated by any other version
		for j, v2 := range allVersions {
			if i == j {
				continue
			}

			comp := v1.value.Version.Compare(v2.value.Version)
			if comp == clock.Before {
				// v1 is dominated by v2
				isDominated = true
				break
			}
		}

		if isDominated {
			// This version is stale
			stale[v1.replicaID] = v1.value
		} else {
			// Check if this is a duplicate of an existing winner
			isDuplicate := false
			for _, winner := range winners {
				if v1.value.Version.Equal(winner.Version) {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				winners = append(winners, v1.value)
			}
		}
	}

	return ReconcileResult{
		Winners: winners,
		Stale:   stale,
	}
}

// HasConflict returns true if there are multiple winners (conflicts).
func (r *ReconcileResult) HasConflict() bool {
	return len(r.Winners) > 1
}

// IsResolved returns true if there's exactly one winner (no conflict).
func (r *ReconcileResult) IsResolved() bool {
	return len(r.Winners) == 1
}

// IsNotFound returns true if there are no winners (all were tombstones or empty).
func (r *ReconcileResult) IsNotFound() bool {
	return len(r.Winners) == 0
}
