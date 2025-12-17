package quorum

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoWrite_Success(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	requiredW := 2

	writeFn := func(ctx context.Context, replicaID string) (bool, error) {
		return true, nil
	}

	result := DoWrite(context.Background(), replicas, requiredW, writeFn)

	if !result.Success {
		t.Errorf("Expected success, got: %v", result.ErrorMessage)
	}
	if result.Acks < requiredW {
		t.Errorf("Expected at least %d acks, got %d", requiredW, result.Acks)
	}
}

func TestDoWrite_QuorumNotMet(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	requiredW := 3

	writeFn := func(ctx context.Context, replicaID string) (bool, error) {
		// Only r1 and r2 succeed
		if replicaID == "r3" {
			return false, errors.New("replica failed")
		}
		return true, nil
	}

	result := DoWrite(context.Background(), replicas, requiredW, writeFn)

	if result.Success {
		t.Error("Expected failure, got success")
	}
	if result.Acks >= requiredW {
		t.Errorf("Expected less than %d acks, got %d", requiredW, result.Acks)
	}
	if result.ErrorMessage == "" {
		t.Error("Expected error message")
	}
}

func TestDoWrite_EarlySuccess(t *testing.T) {
	replicas := []string{"r1", "r2", "r3", "r4", "r5"}
	requiredW := 2

	writeFn := func(ctx context.Context, replicaID string) (bool, error) {
		// Add small delay to test early termination
		time.Sleep(10 * time.Millisecond)
		return true, nil
	}

	start := time.Now()
	result := DoWrite(context.Background(), replicas, requiredW, writeFn)
	duration := time.Since(start)

	if !result.Success {
		t.Errorf("Expected success, got: %v", result.ErrorMessage)
	}

	// Should complete quickly (not wait for all 5 replicas)
	// With W=2, it should return after 2 responses (~20ms) not all 5 (~50ms)
	if duration > 100*time.Millisecond {
		t.Errorf("Expected early termination, took %v", duration)
	}
}

func TestDoRead_Success(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	requiredR := 2

	readFn := func(ctx context.Context, replicaID string) ([]byte, interface{}, bool, error) {
		return []byte("value"), "version", false, nil
	}

	result := DoRead(context.Background(), replicas, requiredR, readFn)

	if !result.Success {
		t.Errorf("Expected success, got: %v", result.ErrorMessage)
	}
	if result.Responses < requiredR {
		t.Errorf("Expected at least %d responses, got %d", requiredR, result.Responses)
	}
	if len(result.Values) < requiredR {
		t.Errorf("Expected at least %d values, got %d", requiredR, len(result.Values))
	}
}

func TestDoRead_QuorumNotMet(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	requiredR := 3

	readFn := func(ctx context.Context, replicaID string) ([]byte, interface{}, bool, error) {
		if replicaID == "r3" {
			return nil, nil, false, errors.New("replica failed")
		}
		return []byte("value"), "version", false, nil
	}

	result := DoRead(context.Background(), replicas, requiredR, readFn)

	if result.Success {
		t.Error("Expected failure, got success")
	}
	if result.Responses >= requiredR {
		t.Errorf("Expected less than %d responses, got %d", requiredR, result.Responses)
	}
}

func TestDoWrite_Timeout(t *testing.T) {
	replicas := []string{"r1", "r2", "r3"}
	requiredW := 2

	writeFn := func(ctx context.Context, replicaID string) (bool, error) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(5 * time.Second):
			return true, nil
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result := DoWrite(ctx, replicas, requiredW, writeFn)

	// Should fail due to timeout
	if result.Success {
		t.Error("Expected failure due to timeout")
	}
}

func TestDoWrite_NoReplicas(t *testing.T) {
	result := DoWrite(context.Background(), []string{}, 2, nil)

	if result.Success {
		t.Error("Expected failure with no replicas")
	}
	if result.ErrorMessage == "" {
		t.Error("Expected error message")
	}
}

func TestDoRead_NoReplicas(t *testing.T) {
	result := DoRead(context.Background(), []string{}, 2, nil)

	if result.Success {
		t.Error("Expected failure with no replicas")
	}
	if result.ErrorMessage == "" {
		t.Error("Expected error message")
	}
}
