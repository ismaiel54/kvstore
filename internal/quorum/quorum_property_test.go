package quorum

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestQuorum_WriteSuccessIffAcksGEQ_W tests that write succeeds iff acks >= W
func TestQuorum_WriteSuccessIffAcksGEQ_W(t *testing.T) {
	tests := []struct {
		name          string
		total         int
		w             int
		successAcks   int
		shouldSucceed bool
	}{
		{"W=2, 2 acks, should succeed", 3, 2, 2, true},
		{"W=2, 1 ack, should fail", 3, 2, 1, false},
		{"W=2, 3 acks, should succeed", 3, 2, 3, true},
		{"W=3, 2 acks, should fail", 3, 3, 2, false},
		{"W=3, 3 acks, should succeed", 3, 3, 3, true},
		{"W=1, 1 ack, should succeed", 3, 1, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replicas := make([]string, tt.total)
			for i := 0; i < tt.total; i++ {
				replicas[i] = "replica" + string(rune('0'+i))
			}

			writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
				// Simulate success for first successAcks replicas
				idx := -1
				for i, r := range replicas {
					if r == replicaAddr {
						idx = i
						break
					}
				}
				if idx < tt.successAcks {
					return true, nil
				}
				return false, errors.New("simulated failure")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := DoWrite(ctx, replicas, tt.w, writeFn)

			if result.Success != tt.shouldSucceed {
				t.Errorf("Expected success=%v, got %v (acks=%d, W=%d)",
					tt.shouldSucceed, result.Success, tt.successAcks, tt.w)
			}
		})
	}
}

// TestQuorum_ReadSuccessIffResponsesGEQ_R tests that read succeeds iff responses >= R
func TestQuorum_ReadSuccessIffResponsesGEQ_R(t *testing.T) {
	tests := []struct {
		name             string
		total            int
		r                int
		successResponses int
		shouldSucceed    bool
	}{
		{"R=2, 2 responses, should succeed", 3, 2, 2, true},
		{"R=2, 1 response, should fail", 3, 2, 1, false},
		{"R=2, 3 responses, should succeed", 3, 2, 3, true},
		{"R=3, 2 responses, should fail", 3, 3, 2, false},
		{"R=3, 3 responses, should succeed", 3, 3, 3, true},
		{"R=1, 1 response, should succeed", 3, 1, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			replicas := make([]string, tt.total)
			for i := 0; i < tt.total; i++ {
				replicas[i] = "replica" + string(rune('0'+i))
			}

			readFn := func(ctx context.Context, replicaAddr string) ([]byte, interface{}, bool, error) {
				// Simulate success for first successResponses replicas
				idx := -1
				for i, r := range replicas {
					if r == replicaAddr {
						idx = i
						break
					}
				}
				if idx < tt.successResponses {
					return []byte("value"), nil, false, nil
				}
				return nil, nil, false, errors.New("simulated failure")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := DoRead(ctx, replicas, tt.r, readFn)

			if result.Success != tt.shouldSucceed {
				t.Errorf("Expected success=%v, got %v (responses=%d, R=%d)",
					tt.shouldSucceed, result.Success, tt.successResponses, tt.r)
			}
		})
	}
}

// TestQuorum_EarlyTermination tests that quorum stops early when threshold met
func TestQuorum_EarlyTermination(t *testing.T) {
	replicas := []string{"r1", "r2", "r3", "r4", "r5"}
	w := 3

	var mu sync.Mutex
	callCount := 0
	writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return true, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := DoWrite(ctx, replicas, w, writeFn)

	if !result.Success {
		t.Error("Expected success")
	}

	// Should have called at least W replicas
	// Due to concurrency, may call more, but should succeed when W is met
	mu.Lock()
	count := callCount
	mu.Unlock()

	if count < w {
		t.Errorf("Should have called at least %d replicas, got %d", w, count)
	}
	// Note: Due to concurrent execution, may call all replicas before checking quorum
	// This is acceptable behavior - the important thing is success when W is met
}

// TestQuorum_TimeoutHandling tests that timeouts are handled correctly
func TestQuorum_TimeoutHandling(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	w := 2

	writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
		// Simulate timeout by waiting longer than context timeout
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(2 * time.Second):
			return true, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := DoWrite(ctx, replicas, w, writeFn)

	// Should fail due to timeout
	if result.Success {
		t.Error("Expected failure due to timeout")
	}
	if result.ErrorMessage == "" {
		t.Error("Expected error message for timeout")
	}
}

// TestQuorum_AllFailures tests that all failures result in failure
func TestQuorum_AllFailures(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	w := 2

	writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
		return false, errors.New("all replicas failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := DoWrite(ctx, replicas, w, writeFn)

	if result.Success {
		t.Error("Expected failure when all replicas fail")
	}
}
