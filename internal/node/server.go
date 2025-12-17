package node

import (
	"time"

	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/repair"
	"kvstore/internal/ring"
	"kvstore/internal/storage"
)

// Server implements the KVStore gRPC service.
type Server struct {
	kvstorepb.UnimplementedKVStoreServer
	store           storage.Store
	nodeID          string
	ring            *ring.Ring
	ringGetter      func() *ring.Ring // Thread-safe ring getter (for dynamic membership)
	selfNode        ring.Node
	clientMgr       *ClientManager
	replicationFactor int
	defaultR        int
	defaultW        int
	readRepairer    *repair.ReadRepairer // Read repair coordinator
}

// NewServer creates a new gRPC server instance.
// If ringGetter is provided, it's used for thread-safe ring access (dynamic membership).
// Otherwise, the static ring is used.
func NewServer(store storage.Store, nodeID string, r *ring.Ring, ringGetter func() *ring.Ring, self ring.Node, clientMgr *ClientManager, rf, defaultR, defaultW int) *Server {
	if rf <= 0 {
		rf = 3
	}
	if defaultR <= 0 {
		defaultR = 2
	}
	if defaultW <= 0 {
		defaultW = 2
	}
	s := &Server{
		store:            store,
		nodeID:           nodeID,
		ring:             r,
		selfNode:         self,
		clientMgr:        clientMgr,
		replicationFactor: rf,
		defaultR:        defaultR,
		defaultW:        defaultW,
	}
	if ringGetter != nil {
		s.ringGetter = ringGetter
	} else {
		s.ringGetter = func() *ring.Ring { return r }
	}
	
	// Initialize read repairer
	s.readRepairer = repair.NewReadRepairer(
		func(addr string) (kvstorepb.KVInternalClient, error) {
			return clientMgr.GetInternalClient(addr)
		},
		2*time.Second, // repair timeout
	)
	
	return s
}

// Put, Get, Delete are implemented in server_quorum.go for Phase 3 quorum coordination

