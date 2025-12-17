package node

import (
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/ring"
	"kvstore/internal/storage"
)

// Server implements the KVStore gRPC service.
type Server struct {
	kvstorepb.UnimplementedKVStoreServer
	store           storage.Store
	nodeID          string
	ring            *ring.Ring
	selfNode        ring.Node
	clientMgr       *ClientManager
	replicationFactor int
	defaultR        int
	defaultW        int
}

// NewServer creates a new gRPC server instance.
func NewServer(store storage.Store, nodeID string, r *ring.Ring, self ring.Node, clientMgr *ClientManager, rf, defaultR, defaultW int) *Server {
	if rf <= 0 {
		rf = 3
	}
	if defaultR <= 0 {
		defaultR = 2
	}
	if defaultW <= 0 {
		defaultW = 2
	}
	return &Server{
		store:            store,
		nodeID:           nodeID,
		ring:             r,
		selfNode:         self,
		clientMgr:        clientMgr,
		replicationFactor: rf,
		defaultR:        defaultR,
		defaultW:        defaultW,
	}
}

// Put, Get, Delete are implemented in server_quorum.go for Phase 3 quorum coordination

