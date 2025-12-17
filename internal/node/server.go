package node

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/grpc/metadata"
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/ring"
	"kvstore/internal/storage"
)

// Server implements the KVStore gRPC service.
type Server struct {
	kvstorepb.UnimplementedKVStoreServer
	store     storage.Store
	nodeID    string
	ring      *ring.Ring
	selfNode  ring.Node
	clientMgr *ClientManager
}

// NewServer creates a new gRPC server instance.
func NewServer(store storage.Store, nodeID string, r *ring.Ring, self ring.Node, clientMgr *ClientManager) *Server {
	return &Server{
		store:     store,
		nodeID:    nodeID,
		ring:      r,
		selfNode:  self,
		clientMgr: clientMgr,
	}
}

// isForwarded checks if the request is already forwarded.
func (s *Server) isForwarded(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(forwardedMetadataKey)
	return len(values) > 0 && values[0] == forwardedValue
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

	// If already forwarded, serve locally
	if s.isForwarded(ctx) {
		return s.putLocal(ctx, req)
	}

	// Route to responsible node
	owner, found := s.ring.ResponsibleNode(req.Key)
	if !found {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: "ring is empty",
		}, nil
	}

	// If owner is self, serve locally
	if owner.ID == s.selfNode.ID {
		return s.putLocal(ctx, req)
	}

	// Forward to owner
	log.Printf("[%s] Forwarding Put for key=%s to %s (%s)", s.nodeID, req.Key, owner.ID, owner.Addr)
	return s.forwardPut(ctx, owner.Addr, req)
}

// putLocal handles Put requests locally.
func (s *Server) putLocal(ctx context.Context, req *kvstorepb.PutRequest) (*kvstorepb.PutResponse, error) {
	version := protoToVectorClock(req.Version)
	newVersion := s.store.Put(req.Key, req.Value, version)

	return &kvstorepb.PutResponse{
		Status:  kvstorepb.PutResponse_SUCCESS,
		Version: vectorClockToProto(newVersion),
	}, nil
}

// forwardPut forwards a Put request to another node.
func (s *Server) forwardPut(ctx context.Context, addr string, req *kvstorepb.PutRequest) (*kvstorepb.PutResponse, error) {
	client, err := s.clientMgr.GetClient(addr)
	if err != nil {
		return &kvstorepb.PutResponse{
			Status:      kvstorepb.PutResponse_ERROR,
			ErrorMessage: fmt.Sprintf("failed to get client: %v", err),
		}, nil
	}

	// Create forwarded context
	md := metadata.New(map[string]string{forwardedMetadataKey: forwardedValue})
	forwardCtx := metadata.NewOutgoingContext(ctx, md)

	return client.Put(forwardCtx, req)
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

	// If already forwarded, serve locally
	if s.isForwarded(ctx) {
		return s.getLocal(ctx, req)
	}

	// Route to responsible node
	owner, found := s.ring.ResponsibleNode(req.Key)
	if !found {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: "ring is empty",
		}, nil
	}

	// If owner is self, serve locally
	if owner.ID == s.selfNode.ID {
		return s.getLocal(ctx, req)
	}

	// Forward to owner
	log.Printf("[%s] Forwarding Get for key=%s to %s (%s)", s.nodeID, req.Key, owner.ID, owner.Addr)
	return s.forwardGet(ctx, owner.Addr, req)
}

// getLocal handles Get requests locally.
func (s *Server) getLocal(ctx context.Context, req *kvstorepb.GetRequest) (*kvstorepb.GetResponse, error) {
	vv := s.store.Get(req.Key)
	if vv == nil {
		return &kvstorepb.GetResponse{
			Status: kvstorepb.GetResponse_NOT_FOUND,
		}, nil
	}

	return &kvstorepb.GetResponse{
		Status: kvstorepb.GetResponse_SUCCESS,
		Value: &kvstorepb.VersionedValue{
			Value:   vv.Value,
			Version: vectorClockToProto(vv.Version),
		},
	}, nil
}

// forwardGet forwards a Get request to another node.
func (s *Server) forwardGet(ctx context.Context, addr string, req *kvstorepb.GetRequest) (*kvstorepb.GetResponse, error) {
	client, err := s.clientMgr.GetClient(addr)
	if err != nil {
		return &kvstorepb.GetResponse{
			Status:       kvstorepb.GetResponse_ERROR,
			ErrorMessage: fmt.Sprintf("failed to get client: %v", err),
		}, nil
	}

	// Create forwarded context
	md := metadata.New(map[string]string{forwardedMetadataKey: forwardedValue})
	forwardCtx := metadata.NewOutgoingContext(ctx, md)

	return client.Get(forwardCtx, req)
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

	// If already forwarded, serve locally
	if s.isForwarded(ctx) {
		return s.deleteLocal(ctx, req)
	}

	// Route to responsible node
	owner, found := s.ring.ResponsibleNode(req.Key)
	if !found {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: "ring is empty",
		}, nil
	}

	// If owner is self, serve locally
	if owner.ID == s.selfNode.ID {
		return s.deleteLocal(ctx, req)
	}

	// Forward to owner
	log.Printf("[%s] Forwarding Delete for key=%s to %s (%s)", s.nodeID, req.Key, owner.ID, owner.Addr)
	return s.forwardDelete(ctx, owner.Addr, req)
}

// deleteLocal handles Delete requests locally.
func (s *Server) deleteLocal(ctx context.Context, req *kvstorepb.DeleteRequest) (*kvstorepb.DeleteResponse, error) {
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

// forwardDelete forwards a Delete request to another node.
func (s *Server) forwardDelete(ctx context.Context, addr string, req *kvstorepb.DeleteRequest) (*kvstorepb.DeleteResponse, error) {
	client, err := s.clientMgr.GetClient(addr)
	if err != nil {
		return &kvstorepb.DeleteResponse{
			Status:       kvstorepb.DeleteResponse_ERROR,
			ErrorMessage: fmt.Sprintf("failed to get client: %v", err),
		}, nil
	}

	// Create forwarded context
	md := metadata.New(map[string]string{forwardedMetadataKey: forwardedValue})
	forwardCtx := metadata.NewOutgoingContext(ctx, md)

	return client.Delete(forwardCtx, req)
}

