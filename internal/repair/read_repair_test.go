package repair

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"kvstore/internal/clock"
	kvstorepb "kvstore/internal/gen/api"
)

// mockInternalClient is a mock for testing read repair
type mockInternalClient struct {
	mu          sync.Mutex
	putCalled   bool
	putKey      string
	putValue    []byte
	putVersion  *kvstorepb.VectorClock
	putDeleted  bool
	putIsRepair bool
	putError    error
}

func (m *mockInternalClient) ReplicaPut(ctx context.Context, req *kvstorepb.ReplicaPutRequest, opts ...grpc.CallOption) (*kvstorepb.ReplicaPutResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.putCalled = true
	m.putKey = req.Key
	m.putValue = req.Value
	m.putVersion = req.Version
	m.putDeleted = req.Deleted
	m.putIsRepair = req.IsRepair

	if m.putError != nil {
		return nil, m.putError
	}

	return &kvstorepb.ReplicaPutResponse{
		Status: kvstorepb.ReplicaPutResponse_SUCCESS,
	}, nil
}

func (m *mockInternalClient) ReplicaGet(ctx context.Context, req *kvstorepb.ReplicaGetRequest, opts ...grpc.CallOption) (*kvstorepb.ReplicaGetResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockInternalClient) ReplicaDelete(ctx context.Context, req *kvstorepb.ReplicaDeleteRequest, opts ...grpc.CallOption) (*kvstorepb.ReplicaDeleteResponse, error) {
	return nil, errors.New("not implemented")
}

func TestReadRepairer_Repair_SingleWinner(t *testing.T) {
	mockClient := &mockInternalClient{}

	repairer := NewReadRepairer(
		func(addr string) (kvstorepb.KVInternalClient, error) {
			return mockClient, nil
		},
		1*time.Second,
	)

	// Create winner version
	vc := clock.New()
	vc.Set("node1", 2)
	vc.Set("node2", 1)

	winners := []VersionedValue{
		{Value: []byte("value"), Version: vc, Deleted: false},
	}

	// Create stale version
	vcStale := clock.New()
	vcStale.Set("node1", 1)

	stale := map[string]VersionedValue{
		"replica1": {Value: []byte("old"), Version: vcStale, Deleted: false},
	}

	replicaIDToAddr := map[string]string{
		"replica1": "127.0.0.1:50052",
	}

	// Trigger repair
	repairer.Repair(context.Background(), "test-key", winners, stale, replicaIDToAddr)

	// Wait a bit for async repair (with retries to avoid race conditions)
	for i := 0; i < 10; i++ {
		time.Sleep(100 * time.Millisecond)
		mockClient.mu.Lock()
		called := mockClient.putCalled
		mockClient.mu.Unlock()
		if called {
			break
		}
	}

	// Verify repair was called
	mockClient.mu.Lock()
	defer mockClient.mu.Unlock()

	if !mockClient.putCalled {
		t.Error("Expected ReplicaPut to be called")
	}
	if mockClient.putKey != "test-key" {
		t.Errorf("Expected key test-key, got %s", mockClient.putKey)
	}
	if string(mockClient.putValue) != "value" {
		t.Errorf("Expected value 'value', got %s", string(mockClient.putValue))
	}
	if !mockClient.putIsRepair {
		t.Error("Expected is_repair to be true")
	}
}

func TestReadRepairer_Repair_NoStale(t *testing.T) {
	mockClient := &mockInternalClient{}

	repairer := NewReadRepairer(
		func(addr string) (kvstorepb.KVInternalClient, error) {
			return mockClient, nil
		},
		1*time.Second,
	)

	winners := []VersionedValue{
		{Value: []byte("value"), Version: clock.New(), Deleted: false},
	}

	stale := map[string]VersionedValue{} // Empty

	repairer.Repair(context.Background(), "test-key", winners, stale, map[string]string{})

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Verify repair was NOT called
	mockClient.mu.Lock()
	defer mockClient.mu.Unlock()

	if mockClient.putCalled {
		t.Error("Expected ReplicaPut NOT to be called when no stale replicas")
	}
}
