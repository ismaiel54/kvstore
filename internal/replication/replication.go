package replication

import (
	"kvstore/internal/ring"
)

// GetReplicasForKey returns the N replicas responsible for a key
// using the ring's preference list.
func GetReplicasForKey(r *ring.Ring, key string, replicationFactor int) []ring.Node {
	if replicationFactor <= 0 {
		replicationFactor = 3 // default
	}
	return r.PreferenceList(key, replicationFactor)
}
