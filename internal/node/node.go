package node

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/ring"
	"kvstore/internal/storage"
)

// Node represents a single node in the distributed system.
type Node struct {
	nodeID     string
	listenAddr string
	grpcServer *grpc.Server
	store      storage.Store
	ring       *ring.Ring
	clientMgr  *ClientManager
	selfNode   ring.Node
	rf         int // replication factor
	r          int // read quorum
	w          int // write quorum
}

// NewNode creates a new node instance.
func NewNode(nodeID, listenAddr string, ringNodes []ring.Node, vnodes, rf, r, w int) *Node {
	store := storage.NewInMemoryStore(nodeID)
	rng := ring.NewRing(vnodes)
	rng.SetNodes(ringNodes)

	selfNode := ring.Node{ID: nodeID, Addr: listenAddr}

	return &Node{
		nodeID:     nodeID,
		listenAddr: listenAddr,
		store:      store,
		ring:       rng,
		clientMgr:  NewClientManager(),
		selfNode:   selfNode,
		rf:         rf,
		r:          r,
		w:          w,
	}
}

// Start starts the gRPC server and begins listening.
func (n *Node) Start() error {
	lis, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", n.listenAddr, err)
	}

	n.grpcServer = grpc.NewServer()
	server := NewServer(n.store, n.nodeID, n.ring, n.selfNode, n.clientMgr, n.rf, n.r, n.w)
	kvstorepb.RegisterKVStoreServer(n.grpcServer, server)
	
	// Register internal service
	internalServer := NewInternalServer(n.store, n.nodeID)
	kvstorepb.RegisterKVInternalServer(n.grpcServer, internalServer)
	
	// Enable gRPC reflection for grpcurl
	reflection.Register(n.grpcServer)

	log.Printf("[%s] Starting node on %s", n.nodeID, n.listenAddr)

	if err := n.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// Stop gracefully stops the node.
func (n *Node) Stop() {
	if n.grpcServer != nil {
		log.Printf("[%s] Stopping node", n.nodeID)
		n.grpcServer.GracefulStop()
	}
}

