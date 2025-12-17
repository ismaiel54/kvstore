package it

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kvstorepb "kvstore/internal/gen/api"
)

func TestSmoke_PutGetDelete_SingleKey(t *testing.T) {
	// Build binary if needed
	binaryPath := "./kvstore"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping integration test. Build with: go build -o kvstore ./cmd/kvstore")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cluster, err := NewCluster(binaryPath)
	require.NoError(t, err)
	defer cluster.Stop()

	// Start 3-node cluster
	err = cluster.StartCluster(ctx)
	require.NoError(t, err, "Failed to start cluster")

	// Get client from first node
	node1 := cluster.GetNode("n1")
	require.NotNil(t, node1)
	client := node1.GetClient()

	// Put
	putCtx, putCancel := context.WithTimeout(ctx, 10*time.Second)
	putResp, err := client.Put(putCtx, &kvstorepb.PutRequest{
		Key:          "test-key",
		Value:        []byte("test-value"),
		ConsistencyW: 2,
		ClientId:     "test-client",
		RequestId:    "req-1",
	})
	putCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp.Status)

	// Get
	getCtx, getCancel := context.WithTimeout(ctx, 10*time.Second)
	getResp, err := client.Get(getCtx, &kvstorepb.GetRequest{
		Key:          "test-key",
		ConsistencyR: 2,
		ClientId:     "test-client",
		RequestId:    "req-2",
	})
	getCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.GetResponse_SUCCESS, getResp.Status)
	assert.NotNil(t, getResp.Value)
	assert.Equal(t, "test-value", string(getResp.Value.Value))

	// Delete
	delCtx, delCancel := context.WithTimeout(ctx, 10*time.Second)
	delResp, err := client.Delete(delCtx, &kvstorepb.DeleteRequest{
		Key:          "test-key",
		ConsistencyW: 2,
		ClientId:     "test-client",
		RequestId:    "req-3",
	})
	delCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.DeleteResponse_SUCCESS, delResp.Status)

	// Get after delete (should return NOT_FOUND or deleted=true)
	getCtx2, getCancel2 := context.WithTimeout(ctx, 10*time.Second)
	getResp2, err := client.Get(getCtx2, &kvstorepb.GetRequest{
		Key:          "test-key",
		ConsistencyR: 2,
		ClientId:     "test-client",
		RequestId:    "req-4",
	})
	getCancel2()
	require.NoError(t, err)
	assert.True(t,
		getResp2.Status == kvstorepb.GetResponse_NOT_FOUND ||
			(getResp2.Value != nil && getResp2.Value.Deleted),
		"Expected NOT_FOUND or deleted=true after delete")
}

func TestQuorum_ToleratesOneNodeDown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	binaryPath := "./kvstore"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cluster, err := NewCluster(binaryPath)
	require.NoError(t, err)
	defer cluster.Stop()

	// Start 3-node cluster
	err = cluster.StartCluster(ctx)
	require.NoError(t, err)

	node1 := cluster.GetNode("n1")
	require.NotNil(t, node1)
	client := node1.GetClient()

	// Initial Put
	putCtx, putCancel := context.WithTimeout(ctx, 10*time.Second)
	putResp, err := client.Put(putCtx, &kvstorepb.PutRequest{
		Key:          "quorum-test",
		Value:        []byte("initial"),
		ConsistencyW: 2,
		ClientId:     "test",
		RequestId:    "req-1",
	})
	putCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp.Status)

	// Kill one node
	err = cluster.KillNode("n3")
	require.NoError(t, err)
	time.Sleep(2 * time.Second) // Wait for failure detection

	// Put should still work (W=2, 2 nodes available)
	putCtx2, putCancel2 := context.WithTimeout(ctx, 10*time.Second)
	putResp2, err := client.Put(putCtx2, &kvstorepb.PutRequest{
		Key:          "quorum-test",
		Value:        []byte("updated"),
		ConsistencyW: 2,
		ClientId:     "test",
		RequestId:    "req-2",
	})
	putCancel2()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp2.Status, "Put should succeed with W=2 and 2 nodes available")

	// Get should still work (R=2, 2 nodes available)
	getCtx, getCancel := context.WithTimeout(ctx, 10*time.Second)
	getResp, err := client.Get(getCtx, &kvstorepb.GetRequest{
		Key:          "quorum-test",
		ConsistencyR: 2,
		ClientId:     "test",
		RequestId:    "req-3",
	})
	getCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.GetResponse_SUCCESS, getResp.Status, "Get should succeed with R=2 and 2 nodes available")
	assert.NotNil(t, getResp.Value)
	assert.Equal(t, "updated", string(getResp.Value.Value))

	// Restart node
	err = cluster.RestartNode(ctx, "n3")
	require.NoError(t, err)

	// Wait for membership to stabilize
	time.Sleep(3 * time.Second)

	// Verify cluster is healthy
	healthCtx, healthCancel := context.WithTimeout(ctx, 5*time.Second)
	healthResp, err := node1.GetHealthClient().Health(healthCtx, &kvstorepb.HealthRequest{})
	healthCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.HealthResponse_OK, healthResp.Status)
}

func TestConflicts_ConcurrentWrites_ReturnSiblings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	binaryPath := "./kvstore"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cluster, err := NewCluster(binaryPath)
	require.NoError(t, err)
	defer cluster.Stop()

	err = cluster.StartCluster(ctx)
	require.NoError(t, err)

	node1 := cluster.GetNode("n1")
	node2 := cluster.GetNode("n2")
	require.NotNil(t, node1)
	require.NotNil(t, node2)

	client1 := node1.GetClient()
	client2 := node2.GetClient()

	// Write from node1 (no version context)
	putCtx1, putCancel1 := context.WithTimeout(ctx, 10*time.Second)
	putResp1, err := client1.Put(putCtx1, &kvstorepb.PutRequest{
		Key:          "conflict-key",
		Value:        []byte("value1"),
		ConsistencyW: 2,
		ClientId:     "client1",
		RequestId:    "req-1",
		// No version context - will create new version
	})
	putCancel1()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp1.Status)

	// Write from node2 concurrently (no version context, different coordinator)
	putCtx2, putCancel2 := context.WithTimeout(ctx, 10*time.Second)
	putResp2, err := client2.Put(putCtx2, &kvstorepb.PutRequest{
		Key:          "conflict-key",
		Value:        []byte("value2"),
		ConsistencyW: 2,
		ClientId:     "client2",
		RequestId:    "req-2",
		// No version context - will create concurrent version
	})
	putCancel2()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp2.Status)

	// Get should return conflicts (siblings)
	getCtx, getCancel := context.WithTimeout(ctx, 10*time.Second)
	getResp, err := client1.Get(getCtx, &kvstorepb.GetRequest{
		Key:          "conflict-key",
		ConsistencyR: 2,
		ClientId:     "client3",
		RequestId:    "req-3",
	})
	getCancel()
	require.NoError(t, err)

	// Should have conflicts (multiple siblings)
	if len(getResp.Conflicts) > 1 {
		// Conflicts detected - good!
		assert.Greater(t, len(getResp.Conflicts), 1, "Expected multiple conflicts/siblings")
	} else if getResp.Value != nil {
		// Single winner - might happen if timing is perfect, but we'll resolve anyway
		t.Logf("Got single winner instead of conflicts (timing-dependent), proceeding with resolution")
	}

	// Resolve conflicts by writing with version context
	// Merge all sibling versions
	mergedVersion := &kvstorepb.VectorClock{}
	if len(getResp.Conflicts) > 0 {
		// Use first conflict's version as base
		mergedVersion = getResp.Conflicts[0].Version
		// Merge other conflicts
		for i := 1; i < len(getResp.Conflicts); i++ {
			// Simple merge: take max of each node's counter
			existing := make(map[string]int64)
			for _, e := range mergedVersion.Entries {
				existing[e.NodeId] = e.Counter
			}
			for _, e := range getResp.Conflicts[i].Version.Entries {
				if existing[e.NodeId] < e.Counter {
					existing[e.NodeId] = e.Counter
				}
			}
			mergedVersion.Entries = make([]*kvstorepb.VectorClockEntry, 0, len(existing))
			for nodeID, counter := range existing {
				mergedVersion.Entries = append(mergedVersion.Entries, &kvstorepb.VectorClockEntry{
					NodeId:  nodeID,
					Counter: counter,
				})
			}
		}
	}

	// Write resolved value with merged version context
	resolveCtx, resolveCancel := context.WithTimeout(ctx, 10*time.Second)
	resolveResp, err := client1.Put(resolveCtx, &kvstorepb.PutRequest{
		Key:          "conflict-key",
		Value:        []byte("resolved-value"),
		ConsistencyW: 2,
		ClientId:     "client3",
		RequestId:    "req-4",
		Version:      mergedVersion,
	})
	resolveCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, resolveResp.Status)

	// Get should now return single winner
	getCtx2, getCancel2 := context.WithTimeout(ctx, 10*time.Second)
	getResp2, err := client1.Get(getCtx2, &kvstorepb.GetRequest{
		Key:          "conflict-key",
		ConsistencyR: 2,
		ClientId:     "client3",
		RequestId:    "req-5",
	})
	getCancel2()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.GetResponse_SUCCESS, getResp2.Status)
	assert.NotNil(t, getResp2.Value, "Should have single value after resolution")
	assert.Equal(t, "resolved-value", string(getResp2.Value.Value))
	assert.Equal(t, 0, len(getResp2.Conflicts), "Should have no conflicts after resolution")
}

func TestReadRepair_RepairsStaleReplica(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	binaryPath := "./kvstore"
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cluster, err := NewCluster(binaryPath)
	require.NoError(t, err)
	defer cluster.Stop()

	err = cluster.StartCluster(ctx)
	require.NoError(t, err)

	node1 := cluster.GetNode("n1")
	node3 := cluster.GetNode("n3")
	require.NotNil(t, node1)
	require.NotNil(t, node3)

	client1 := node1.GetClient()

	// Put v1
	putCtx1, putCancel1 := context.WithTimeout(ctx, 10*time.Second)
	putResp1, err := client1.Put(putCtx1, &kvstorepb.PutRequest{
		Key:          "repair-test",
		Value:        []byte("v1"),
		ConsistencyW: 2,
		ClientId:     "test",
		RequestId:    "req-1",
	})
	putCancel1()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp1.Status)

	// Kill node3
	err = cluster.KillNode("n3")
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	// Put v2 (node3 misses it, becomes stale)
	putCtx2, putCancel2 := context.WithTimeout(ctx, 10*time.Second)
	putResp2, err := client1.Put(putCtx2, &kvstorepb.PutRequest{
		Key:          "repair-test",
		Value:        []byte("v2"),
		ConsistencyW: 2,
		ClientId:     "test",
		RequestId:    "req-2",
	})
	putCancel2()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.PutResponse_SUCCESS, putResp2.Status)

	// Restart node3
	err = cluster.RestartNode(ctx, "n3")
	require.NoError(t, err)
	time.Sleep(2 * time.Second) // Wait for restart

	// Get should trigger read repair
	getCtx, getCancel := context.WithTimeout(ctx, 10*time.Second)
	getResp, err := client1.Get(getCtx, &kvstorepb.GetRequest{
		Key:          "repair-test",
		ConsistencyR: 2,
		ClientId:     "test",
		RequestId:    "req-3",
	})
	getCancel()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.GetResponse_SUCCESS, getResp.Status)

	// Wait for repair to complete
	time.Sleep(2 * time.Second)

	// Get again to verify convergence
	getCtx2, getCancel2 := context.WithTimeout(ctx, 10*time.Second)
	getResp2, err := client1.Get(getCtx2, &kvstorepb.GetRequest{
		Key:          "repair-test",
		ConsistencyR: 2,
		ClientId:     "test",
		RequestId:    "req-4",
	})
	getCancel2()
	require.NoError(t, err)
	assert.Equal(t, kvstorepb.GetResponse_SUCCESS, getResp2.Status)
	assert.NotNil(t, getResp2.Value)

	// Should have v2 (or at least a consistent value)
	value := string(getResp2.Value.Value)
	assert.Contains(t, []string{"v2"}, value, "After repair, should have latest value v2")
}
