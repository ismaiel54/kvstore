package node

import (
	"context"
	"log"

	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/storage"
)

// InternalServer implements the KVInternal gRPC service for replica operations.
type InternalServer struct {
	kvstorepb.UnimplementedKVInternalServer
	store  storage.Store
	nodeID string
}

// NewInternalServer creates a new internal server instance.
func NewInternalServer(store storage.Store, nodeID string) *InternalServer {
	return &InternalServer{
		store:  store,
		nodeID: nodeID,
	}
}

// ReplicaPut handles internal Put requests from coordinator to replica.
func (s *InternalServer) ReplicaPut(ctx context.Context, req *kvstorepb.ReplicaPutRequest) (*kvstorepb.ReplicaPutResponse, error) {
	log.Printf("[%s] ReplicaPut: key=%s, coordinator=%s, request_id=%s",
		s.nodeID, req.Key, req.CoordinatorId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.ReplicaPutResponse{
			Status:       kvstorepb.ReplicaPutResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Convert protobuf version to internal version
	version := protoToVectorClock(req.Version)

	// If this is a repair operation, do NOT increment clock
	// Just overwrite with the provided version
	if req.IsRepair {
		// For repair: overwrite with exact version (no increment)
		// Storage should accept if incoming version dominates or is equal
		err := s.store.PutRepair(req.Key, req.Value, version, req.Deleted)
		if err != nil {
			return &kvstorepb.ReplicaPutResponse{
				Status:       kvstorepb.ReplicaPutResponse_ERROR,
				ErrorMessage: err.Error(),
			}, nil
		}
		return &kvstorepb.ReplicaPutResponse{
			Status: kvstorepb.ReplicaPutResponse_SUCCESS,
		}, nil
	}

	// Normal operation: store and increment
	newVersion := s.store.Put(req.Key, req.Value, version, req.Deleted)

	// Verify version was updated
	_ = newVersion

	return &kvstorepb.ReplicaPutResponse{
		Status: kvstorepb.ReplicaPutResponse_SUCCESS,
	}, nil
}

// ReplicaGet handles internal Get requests from coordinator to replica.
func (s *InternalServer) ReplicaGet(ctx context.Context, req *kvstorepb.ReplicaGetRequest) (*kvstorepb.ReplicaGetResponse, error) {
	log.Printf("[%s] ReplicaGet: key=%s, coordinator=%s, request_id=%s",
		s.nodeID, req.Key, req.CoordinatorId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.ReplicaGetResponse{
			Status:       kvstorepb.ReplicaGetResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	vv := s.store.Get(req.Key)
	if vv == nil {
		return &kvstorepb.ReplicaGetResponse{
			Status: kvstorepb.ReplicaGetResponse_NOT_FOUND,
		}, nil
	}

	return &kvstorepb.ReplicaGetResponse{
		Status: kvstorepb.ReplicaGetResponse_SUCCESS,
		Value: &kvstorepb.VersionedValue{
			Value:   vv.Value,
			Version: vectorClockToProto(vv.Version),
			Deleted: vv.Deleted,
		},
	}, nil
}

// ReplicaDelete handles internal Delete requests from coordinator to replica.
func (s *InternalServer) ReplicaDelete(ctx context.Context, req *kvstorepb.ReplicaDeleteRequest) (*kvstorepb.ReplicaDeleteResponse, error) {
	log.Printf("[%s] ReplicaDelete: key=%s, coordinator=%s, request_id=%s",
		s.nodeID, req.Key, req.CoordinatorId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.ReplicaDeleteResponse{
			Status:       kvstorepb.ReplicaDeleteResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Convert protobuf version to internal version
	version := protoToVectorClock(req.Version)

	// Delete the key (stores tombstone)
	newVersion := s.store.Delete(req.Key, version)

	// Verify version was updated
	_ = newVersion

	return &kvstorepb.ReplicaDeleteResponse{
		Status: kvstorepb.ReplicaDeleteResponse_SUCCESS,
	}, nil
}
