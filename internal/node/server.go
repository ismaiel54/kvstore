package node

import (
	"context"
	"log"

	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/storage"
)

// Server implements the KVStore gRPC service.
type Server struct {
	kvstorepb.UnimplementedKVStoreServer
	store  storage.Store
	nodeID string
}

// NewServer creates a new gRPC server instance.
func NewServer(store storage.Store, nodeID string) *Server {
	return &Server{
		store:  store,
		nodeID: nodeID,
	}
}

// Put handles Put requests.
func (s *Server) Put(ctx context.Context, req *kvstorepb.PutRequest) (*kvstorepb.PutResponse, error) {
	log.Printf("[%s] Put request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Convert protobuf version to internal version
	version := protoToVectorClock(req.Version)

	// Store the value
	newVersion := s.store.Put(req.Key, req.Value, version)

	return &kvstorepb.PutResponse{
		Status:  kvstorepb.PutResponse_SUCCESS,
		Version: vectorClockToProto(newVersion),
	}, nil
}

// Get handles Get requests.
func (s *Server) Get(ctx context.Context, req *kvstorepb.GetRequest) (*kvstorepb.GetResponse, error) {
	log.Printf("[%s] Get request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	vv := s.store.Get(req.Key)
	if vv == nil {
		return &kvstorepb.GetResponse{
			Status: kvstorepb.GetResponse_NOT_FOUND,
		}, nil
	}

	// In Phase 1, we only return a single value.
	// Conflict resolution will be added in Phase 4.
	return &kvstorepb.GetResponse{
		Status: kvstorepb.GetResponse_SUCCESS,
		Value: &kvstorepb.VersionedValue{
			Value:   vv.Value,
			Version: vectorClockToProto(vv.Version),
		},
	}, nil
}

// Delete handles Delete requests.
func (s *Server) Delete(ctx context.Context, req *kvstorepb.DeleteRequest) (*kvstorepb.DeleteResponse, error) {
	log.Printf("[%s] Delete request: key=%s, client_id=%s, request_id=%s",
		s.nodeID, req.Key, req.ClientId, req.RequestId)

	if req.Key == "" {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: "key cannot be empty",
		}, nil
	}

	// Convert protobuf version to internal version
	version := protoToVectorClock(req.Version)

	// Check if key exists before deleting
	existing := s.store.Get(req.Key)
	if existing == nil {
		return &kvstorepb.DeleteResponse{
			Status: kvstorepb.DeleteResponse_NOT_FOUND,
		}, nil
	}

	// Delete the key
	newVersion := s.store.Delete(req.Key, version)

	return &kvstorepb.DeleteResponse{
		Status:  kvstorepb.DeleteResponse_SUCCESS,
		Version: vectorClockToProto(newVersion),
	}, nil
}

