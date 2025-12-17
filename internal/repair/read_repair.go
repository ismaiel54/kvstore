package repair

import (
	"context"
	"fmt"
	"log"
	"time"

	"kvstore/internal/clock"
	kvstorepb "kvstore/internal/gen/api"
)

// ReadRepairer performs asynchronous read repair to converge stale replicas.
type ReadRepairer struct {
	// clientProvider returns an internal client for a given node address
	clientProvider func(addr string) (kvstorepb.KVInternalClient, error)
	timeout        time.Duration
}

// NewReadRepairer creates a new read repairer.
func NewReadRepairer(clientProvider func(addr string) (kvstorepb.KVInternalClient, error), timeout time.Duration) *ReadRepairer {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &ReadRepairer{
		clientProvider: clientProvider,
		timeout:        timeout,
	}
}

// Repair asynchronously repairs stale replicas with winning versions.
// This is fire-and-forget: it logs errors but does not block or retry.
// ctx should be detached from the request context (use context.Background()).
func (r *ReadRepairer) Repair(ctx context.Context, key string, winners []VersionedValue, stale map[string]VersionedValue, replicaIDToAddr map[string]string) {
	if len(stale) == 0 {
		return // Nothing to repair
	}

	// Fire-and-forget: run in goroutine
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Read repair panic for key %s: %v", key, err)
			}
		}()

		// Use detached context with timeout
		repairCtx, cancel := context.WithTimeout(context.Background(), r.timeout)
		defer cancel()

		log.Printf("Read repair triggered for key=%s: %d stale replicas, %d winners", key, len(stale), len(winners))

		repairCount := 0
		failureCount := 0

		for replicaID, staleValue := range stale {
			// Skip if we don't have address mapping
			addr, exists := replicaIDToAddr[replicaID]
			if !exists {
				log.Printf("Read repair: skipping replica %s (no address)", replicaID)
				continue
			}

			// Repair this replica
			if err := r.repairReplica(repairCtx, addr, key, winners, staleValue); err != nil {
				log.Printf("Read repair failed for replica %s (key=%s): %v", replicaID, key, err)
				failureCount++
			} else {
				repairCount++
			}
		}

		log.Printf("Read repair completed for key=%s: %d repaired, %d failed", key, repairCount, failureCount)
	}()
}

// repairReplica repairs a single stale replica with winning versions.
func (r *ReadRepairer) repairReplica(ctx context.Context, addr string, key string, winners []VersionedValue, staleValue VersionedValue) error {
	client, err := r.clientProvider(addr)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// If single winner, write that version
	if len(winners) == 1 {
		winner := winners[0]
		return r.writeVersion(ctx, client, key, winner)
	}

	// Multiple winners (siblings): write all of them
	// In Dynamo-style, we write all siblings so replica converges to same conflict state
	// Note: This is a simplification - in practice, we might merge or choose one
	// For now, we write the first winner (dominant if any, otherwise first concurrent)
	// A more complete implementation would write all siblings, but that requires
	// storage to support multiple concurrent versions per key (out of scope)

	// For simplicity, write the first winner
	// In a full implementation, we'd need storage to support sibling sets
	if len(winners) > 0 {
		return r.writeVersion(ctx, client, key, winners[0])
	}

	return fmt.Errorf("no winners to repair with")
}

// writeVersion writes a version to a replica (put or delete/tombstone).
func (r *ReadRepairer) writeVersion(ctx context.Context, client kvstorepb.KVInternalClient, key string, vv VersionedValue) error {
	req := &kvstorepb.ReplicaPutRequest{
		Key:           key,
		Value:         vv.Value,
		Version:       vectorClockToProto(vv.Version),
		CoordinatorId: "read-repair", // Special ID for repair operations
		RequestId:     fmt.Sprintf("repair-%d", time.Now().UnixNano()),
		Deleted:       vv.Deleted,
		IsRepair:      true, // Mark as repair to prevent clock increments
	}

	resp, err := client.ReplicaPut(ctx, req)
	if err != nil {
		return fmt.Errorf("replica put failed: %w", err)
	}

	if resp.Status != kvstorepb.ReplicaPutResponse_SUCCESS {
		return fmt.Errorf("replica put returned error: %s", resp.ErrorMessage)
	}

	return nil
}

// vectorClockToProto converts a vector clock to protobuf format.
func vectorClockToProto(vc clock.VectorClock) *kvstorepb.VectorClock {
	if vc == nil {
		return &kvstorepb.VectorClock{}
	}

	entries := make([]*kvstorepb.VectorClockEntry, 0)
	for nodeID, counter := range vc {
		entries = append(entries, &kvstorepb.VectorClockEntry{
			NodeId:  nodeID,
			Counter: int64(counter),
		})
	}

	return &kvstorepb.VectorClock{
		Entries: entries,
	}
}
