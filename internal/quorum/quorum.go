package quorum

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// DefaultPerReplicaTimeout is the default timeout for each replica RPC.
	DefaultPerReplicaTimeout = 2 * time.Second
)

// WriteResult represents the result of a quorum write operation.
type WriteResult struct {
	Success      bool
	Acks         int
	Required     int
	Replicas     int
	ErrorMessage string
}

// ReadResult represents the result of a quorum read operation.
type ReadResult struct {
	Success      bool
	Responses    int
	Required     int
	Replicas     int
	Values       []ReadValue
	ErrorMessage string
}

// ReadValue represents a value read from a replica.
type ReadValue struct {
	Value   []byte
	Version interface{} // Will be clock.VectorClock, but using interface{} to avoid circular import
	Deleted bool
}

// ReplicaWriteFunc is a function that performs a write to a single replica.
// Returns true if successful, false otherwise.
type ReplicaWriteFunc func(ctx context.Context, replicaID string) (bool, error)

// ReplicaReadFunc is a function that performs a read from a single replica.
// Returns the value, version, deleted flag, and error.
type ReplicaReadFunc func(ctx context.Context, replicaID string) ([]byte, interface{}, bool, error)

// DoWrite performs a quorum write operation.
// It fans out to all replicas in parallel and returns success when W acks are received.
func DoWrite(ctx context.Context, replicas []string, requiredW int, writeFn ReplicaWriteFunc) WriteResult {
	if len(replicas) == 0 {
		return WriteResult{
			Success:      false,
			ErrorMessage: "no replicas provided",
		}
	}

	if requiredW <= 0 {
		requiredW = (len(replicas) / 2) + 1 // default: majority
	}

	if requiredW > len(replicas) {
		return WriteResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("required W=%d exceeds replica count=%d", requiredW, len(replicas)),
		}
	}

	var (
		mu       sync.Mutex
		acks     int
		errors   []error
		wg       sync.WaitGroup
	)

	// Create context with per-replica timeout
	replicaCtx, cancel := context.WithTimeout(ctx, DefaultPerReplicaTimeout)
	defer cancel()

	// Fanout to all replicas
	for _, replicaID := range replicas {
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()

			success, err := writeFn(replicaCtx, rid)
			mu.Lock()
			defer mu.Unlock()

			if success {
				acks++
			} else if err != nil {
				errors = append(errors, fmt.Errorf("replica %s: %w", rid, err))
			}
		}(replicaID)
	}

	// Wait for quorum or all responses
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All replicas responded
	case <-ctx.Done():
		// Parent context cancelled
		return WriteResult{
			Success:      false,
			Acks:          acks,
			Required:     requiredW,
			Replicas:     len(replicas),
			ErrorMessage: fmt.Sprintf("context cancelled: %v", ctx.Err()),
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if acks >= requiredW {
		return WriteResult{
			Success:  true,
			Acks:     acks,
			Required: requiredW,
			Replicas: len(replicas),
		}
	}

	// Quorum not met
	errMsg := fmt.Sprintf("quorum not met: acks=%d required=%d replicas=%d", acks, requiredW, len(replicas))
	if len(errors) > 0 {
		errMsg += fmt.Sprintf(" errors=%v", errors[:min(3, len(errors))])
	}

	return WriteResult{
		Success:      false,
		Acks:          acks,
		Required:     requiredW,
		Replicas:     len(replicas),
		ErrorMessage: errMsg,
	}
}

// DoRead performs a quorum read operation.
// It fans out to all replicas in parallel and returns when R responses are received.
func DoRead(ctx context.Context, replicas []string, requiredR int, readFn ReplicaReadFunc) ReadResult {
	if len(replicas) == 0 {
		return ReadResult{
			Success:      false,
			ErrorMessage: "no replicas provided",
		}
	}

	if requiredR <= 0 {
		requiredR = (len(replicas) / 2) + 1 // default: majority
	}

	if requiredR > len(replicas) {
		return ReadResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("required R=%d exceeds replica count=%d", requiredR, len(replicas)),
		}
	}

	var (
		mu        sync.Mutex
		responses int
		values    []ReadValue
		errors    []error
		wg        sync.WaitGroup
	)

	// Create context with per-replica timeout
	replicaCtx, cancel := context.WithTimeout(ctx, DefaultPerReplicaTimeout)
	defer cancel()

	// Fanout to all replicas
	for _, replicaID := range replicas {
		wg.Add(1)
		go func(rid string) {
			defer wg.Done()

			value, version, deleted, err := readFn(replicaCtx, rid)
			mu.Lock()
			defer mu.Unlock()

			if err == nil {
				responses++
				values = append(values, ReadValue{
					Value:   value,
					Version: version,
					Deleted: deleted,
				})
			} else {
				errors = append(errors, fmt.Errorf("replica %s: %w", rid, err))
			}
		}(replicaID)
	}

	// Wait for quorum or all responses
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All replicas responded
	case <-ctx.Done():
		// Parent context cancelled
		return ReadResult{
			Success:      false,
			Responses:    responses,
			Required:     requiredR,
			Replicas:     len(replicas),
			ErrorMessage: fmt.Sprintf("context cancelled: %v", ctx.Err()),
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if responses >= requiredR {
		return ReadResult{
			Success:   true,
			Responses: responses,
			Required:  requiredR,
			Replicas:  len(replicas),
			Values:    values,
		}
	}

	// Quorum not met
	errMsg := fmt.Sprintf("quorum not met: responses=%d required=%d replicas=%d", responses, requiredR, len(replicas))
	if len(errors) > 0 {
		errMsg += fmt.Sprintf(" errors=%v", errors[:min(3, len(errors))])
	}

	return ReadResult{
		Success:      false,
		Responses:    responses,
		Required:     requiredR,
		Replicas:     len(replicas),
		ErrorMessage: errMsg,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

