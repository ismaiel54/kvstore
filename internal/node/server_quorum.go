package node

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/clock"
	"kvstore/internal/quorum"
	"kvstore/internal/replication"
	"kvstore/internal/repair"
	"kvstore/internal/ring"
)

// Put handles Put requests with quorum coordination.
func (s *Server) Put(ctx context.Context, req *kvstorepb.PutRequest) (*kvstorepb.PutResponse, error) {
	log.Printf("[%s] Put request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Get replication factor and quorum sizes
	rf := s.replicationFactor
	if rf <= 0 {
		rf = 3
	}
	requiredW := int(req.ConsistencyW)
	if requiredW <= 0 {
		requiredW = s.defaultW
	}

	// Get preference list (replicas)
	replicas := replication.GetReplicasForKey(s.ring, req.Key, rf)
	if len(replicas) == 0 {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: "no replicas available",
		}, nil
	}

	// Prepare version: merge client-provided context (known versions from previous Get)
	// This enables proper causality when client resolves conflicts
	newVersion := clock.New()
	if req.Version != nil {
		// Client provided a version context (e.g., from resolving conflicts)
		// Merge it to ensure the new write dominates the known versions
		newVersion = protoToVectorClock(req.Version)
	}
	// Increment coordinator's counter to create new version
	newVersion.Increment(s.nodeID)

	// Convert replicas to string IDs for quorum coordinator
	replicaIDs := make([]string, len(replicas))
	for i, r := range replicas {
		replicaIDs[i] = r.Addr
	}

	// Perform quorum write
	writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
		// Find replica node
		var replicaNode ring.Node
		for _, r := range replicas {
			if r.Addr == replicaAddr {
				replicaNode = r
				break
			}
		}

		// If replica is self, write locally
		if replicaNode.ID == s.selfNode.ID {
			s.store.Put(req.Key, req.Value, newVersion, false)
			return true, nil
		}

		// Otherwise, call internal RPC
		client, err := s.clientMgr.GetInternalClient(replicaAddr)
		if err != nil {
			return false, fmt.Errorf("failed to get internal client: %w", err)
		}

		replicaReq := &kvstorepb.ReplicaPutRequest{
			Key:           req.Key,
			Value:         req.Value,
			Version:       vectorClockToProto(newVersion),
			CoordinatorId: s.nodeID,
			RequestId:     req.RequestId,
			Deleted:       false,
		}

		resp, err := client.ReplicaPut(ctx, replicaReq)
		if err != nil {
			return false, err
		}

		return resp.Status == kvstorepb.ReplicaPutResponse_SUCCESS, nil
	}

	result := quorum.DoWrite(ctx, replicaIDs, requiredW, writeFn)

	if !result.Success {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: result.ErrorMessage,
		}, status.Error(codes.Unavailable, result.ErrorMessage)
	}

	return &kvstorepb.PutResponse{
		Status:  kvstorepb.PutResponse_SUCCESS,
		Version: vectorClockToProto(newVersion),
	}, nil
}

// Get handles Get requests with quorum coordination.
func (s *Server) Get(ctx context.Context, req *kvstorepb.GetRequest) (*kvstorepb.GetResponse, error) {
	log.Printf("[%s] Get request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Get replication factor and quorum sizes
	rf := s.replicationFactor
	if rf <= 0 {
		rf = 3
	}
	requiredR := int(req.ConsistencyR)
	if requiredR <= 0 {
		requiredR = s.defaultR
	}

	// Get preference list (replicas)
	replicas := replication.GetReplicasForKey(s.ring, req.Key, rf)
	if len(replicas) == 0 {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: "no replicas available",
		}, nil
	}

	// Convert replicas to addresses for quorum coordinator
	replicaAddrs := make([]string, len(replicas))
	replicaIDMap := make(map[string]string) // addr -> nodeID
	for i, r := range replicas {
		replicaAddrs[i] = r.Addr
		replicaIDMap[r.Addr] = r.ID
	}

	// Perform quorum read
	readFn := func(ctx context.Context, replicaAddr string) ([]byte, interface{}, bool, error) {
		// Find replica node
		var replicaNode ring.Node
		for _, r := range replicas {
			if r.Addr == replicaAddr {
				replicaNode = r
				break
			}
		}

		// If replica is self, read locally
		if replicaNode.ID == s.selfNode.ID {
			vv := s.store.Get(req.Key)
			if vv == nil {
				return nil, nil, false, fmt.Errorf("not found")
			}
			return vv.Value, vv.Version, vv.Deleted, nil
		}

		// Otherwise, call internal RPC
		client, err := s.clientMgr.GetInternalClient(replicaAddr)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to get internal client: %w", err)
		}

		replicaReq := &kvstorepb.ReplicaGetRequest{
			Key:           req.Key,
			CoordinatorId: s.nodeID,
			RequestId:     req.RequestId,
		}

		resp, err := client.ReplicaGet(ctx, replicaReq)
		if err != nil {
			return nil, nil, false, err
		}

		if resp.Status == kvstorepb.ReplicaGetResponse_NOT_FOUND {
			return nil, nil, false, fmt.Errorf("not found")
		}

		if resp.Status != kvstorepb.ReplicaGetResponse_SUCCESS {
			return nil, nil, false, fmt.Errorf("replica error: %s", resp.ErrorMessage)
		}

		version := protoToVectorClock(resp.Value.Version)
		deleted := resp.Value.Deleted
		return resp.Value.Value, version, deleted, nil
	}

	result := quorum.DoRead(ctx, replicaAddrs, requiredR, readFn)

	if !result.Success {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: result.ErrorMessage,
		}, status.Error(codes.Unavailable, result.ErrorMessage)
	}

	// Reconcile versions using proper algorithm
	if len(result.Values) == 0 {
		return &kvstorepb.GetResponse{
			Status: kvstorepb.GetResponse_NOT_FOUND,
		}, nil
	}

	// Convert ReadValue to repair.VersionedValue for reconciliation
	// Note: quorum.DoRead doesn't preserve replica addresses, so we'll use indices
	repairValues := make([]repair.VersionedValue, 0, len(result.Values))
	replicaIDs := make([]string, 0, len(result.Values))

	for i, rv := range result.Values {
		vc, ok := rv.Version.(clock.VectorClock)
		if !ok {
			continue
		}
		repairValues = append(repairValues, repair.VersionedValue{
			Value:   rv.Value,
			Version: vc,
			Deleted: rv.Deleted,
		})
		// Use index to map back to replica (approximate, but sufficient for reconciliation)
		if i < len(replicas) {
			replicaIDs = append(replicaIDs, replicas[i].ID)
		} else {
			replicaIDs = append(replicaIDs, fmt.Sprintf("replica-%d", i))
		}
	}

	// Use reconcile algorithm to compute maximal set
	reconcileResult := repair.Reconcile(repairValues, replicaIDs)

	// Handle results
	if reconcileResult.IsNotFound() {
		return &kvstorepb.GetResponse{
			Status: kvstorepb.GetResponse_NOT_FOUND,
		}, nil
	}

	if reconcileResult.IsResolved() {
		// Single winner - return it
		winner := reconcileResult.Winners[0]
		if winner.Deleted {
			// Tombstone - return as NOT_FOUND
			return &kvstorepb.GetResponse{
				Status: kvstorepb.GetResponse_NOT_FOUND,
			}, nil
		}
		return &kvstorepb.GetResponse{
			Status: kvstorepb.GetResponse_SUCCESS,
			Value: &kvstorepb.VersionedValue{
				Value:   winner.Value,
				Version: vectorClockToProto(winner.Version),
				Deleted: winner.Deleted,
			},
		}, nil
	}

	// Multiple winners (conflicts) - return siblings
	conflicts := make([]*kvstorepb.VersionedValue, 0, len(reconcileResult.Winners))
	for _, winner := range reconcileResult.Winners {
		conflicts = append(conflicts, &kvstorepb.VersionedValue{
			Value:   winner.Value,
			Version: vectorClockToProto(winner.Version),
			Deleted: winner.Deleted,
		})
	}

	return &kvstorepb.GetResponse{
		Status:    kvstorepb.GetResponse_SUCCESS,
		Conflicts: conflicts,
	}, nil
}

// Delete handles Delete requests with quorum coordination.
func (s *Server) Delete(ctx context.Context, req *kvstorepb.DeleteRequest) (*kvstorepb.DeleteResponse, error) {
	log.Printf("[%s] Delete request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Get replication factor and quorum sizes
	rf := s.replicationFactor
	if rf <= 0 {
		rf = 3
	}
	requiredW := int(req.ConsistencyW)
	if requiredW <= 0 {
		requiredW = s.defaultW
	}

	// Get preference list (replicas)
	replicas := replication.GetReplicasForKey(s.ring, req.Key, rf)
	if len(replicas) == 0 {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: "no replicas available",
		}, nil
	}

	// Prepare version
	newVersion := clock.New()
	if req.Version != nil {
		newVersion = protoToVectorClock(req.Version)
	}
	newVersion.Increment(s.nodeID)

	// Convert replicas to string IDs for quorum coordinator
	replicaIDs := make([]string, len(replicas))
	for i, r := range replicas {
		replicaIDs[i] = r.Addr
	}

	// Perform quorum write (tombstone)
	writeFn := func(ctx context.Context, replicaAddr string) (bool, error) {
		// Find replica node
		var replicaNode ring.Node
		for _, r := range replicas {
			if r.Addr == replicaAddr {
				replicaNode = r
				break
			}
		}

		// If replica is self, write tombstone locally
		if replicaNode.ID == s.selfNode.ID {
			s.store.Put(req.Key, nil, newVersion, true) // deleted=true
			return true, nil
		}

		// Otherwise, call internal RPC
		client, err := s.clientMgr.GetInternalClient(replicaAddr)
		if err != nil {
			return false, fmt.Errorf("failed to get internal client: %w", err)
		}

		replicaReq := &kvstorepb.ReplicaPutRequest{
			Key:           req.Key,
			Value:         nil,
			Version:       vectorClockToProto(newVersion),
			CoordinatorId: s.nodeID,
			RequestId:     req.RequestId,
			Deleted:       true,
		}

		resp, err := client.ReplicaPut(ctx, replicaReq)
		if err != nil {
			return false, err
		}

		return resp.Status == kvstorepb.ReplicaPutResponse_SUCCESS, nil
	}

	result := quorum.DoWrite(ctx, replicaIDs, requiredW, writeFn)

	if !result.Success {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: result.ErrorMessage,
		}, status.Error(codes.Unavailable, result.ErrorMessage)
	}

	return &kvstorepb.DeleteResponse{
		Status:  kvstorepb.DeleteResponse_SUCCESS,
		Version: vectorClockToProto(newVersion),
	}, nil
}

