package node

import (
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/storage"
)

// Node represents a single node in the distributed system.
type Node struct {
	nodeID      string
	listenAddr  string
	grpcServer  *grpc.Server
	store       storage.Store
}

// NewNode creates a new node instance.
func NewNode(nodeID, listenAddr string) *Node {
	store := storage.NewInMemoryStore(nodeID)
	return &Node{
		nodeID:     nodeID,
		listenAddr: listenAddr,
		store:      store,
	}
}

// Start starts the gRPC server and begins listening.
func (n *Node) Start() error {
	lis, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", n.listenAddr, err)
	}

	n.grpcServer = grpc.NewServer()
	server := NewServer(n.store, n.nodeID)
	kvstorepb.RegisterKVStoreServer(n.grpcServer, server)
	
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

