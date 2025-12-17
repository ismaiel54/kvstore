package node

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/gossip"
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
	ringMu     sync.RWMutex // Protects ring updates
	clientMgr  *ClientManager
	selfNode   ring.Node
	rf         int // replication factor
	r          int // read quorum
	w          int // write quorum
	membership *gossip.Membership
}

// NewNode creates a new node instance.
// If seeds is non-empty, uses gossip membership. Otherwise, uses static ringNodes.
func NewNode(nodeID, listenAddr string, ringNodes []ring.Node, seeds []ring.Node, vnodes, rf, r, w int) *Node {
	store := storage.NewInMemoryStore(nodeID)
	rng := ring.NewRing(vnodes)
	selfNode := ring.Node{ID: nodeID, Addr: listenAddr}

	n := &Node{
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

	// Initialize membership if seeds provided (dynamic), otherwise use static
	if len(seeds) > 0 {
		// Dynamic membership with gossip
		membership := gossip.NewMembership(nodeID, listenAddr, 1*time.Second, 3*time.Second, 10*time.Second)
		membership.AddSeedMembers(seeds)
		membership.SetOnMembershipChanged(n.onMembershipChanged)
		n.membership = membership
		// Initial ring from seeds + self
		initialNodes := append([]ring.Node{selfNode}, seeds...)
		rng.SetNodes(initialNodes)
	} else {
		// Static membership
		rng.SetNodes(ringNodes)
	}

	return n
}

// Start starts the gRPC server and begins listening.
func (n *Node) Start() error {
	lis, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", n.listenAddr, err)
	}

	n.grpcServer = grpc.NewServer()
	
	// Create thread-safe ring getter
	ringGetter := func() *ring.Ring {
		n.ringMu.RLock()
		defer n.ringMu.RUnlock()
		return n.ring
	}
	
	server := NewServer(n.store, n.nodeID, n.ring, ringGetter, n.selfNode, n.clientMgr, n.rf, n.r, n.w)
	kvstorepb.RegisterKVStoreServer(n.grpcServer, server)
	
	// Register internal service
	internalServer := NewInternalServer(n.store, n.nodeID)
	kvstorepb.RegisterKVInternalServer(n.grpcServer, internalServer)
	
	// Register membership service if using gossip
	if n.membership != nil {
		membershipServer := gossip.NewServer(n.membership)
		kvstorepb.RegisterMembershipServer(n.grpcServer, membershipServer)
		
		// Start membership protocol
		n.membership.Start(n.probeFn, n.gossipFn)
		log.Printf("[%s] Started gossip membership", n.nodeID)
	}
	
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
	if n.membership != nil {
		n.membership.Stop()
	}
	if n.grpcServer != nil {
		log.Printf("[%s] Stopping node", n.nodeID)
		n.grpcServer.GracefulStop()
	}
}

// onMembershipChanged is called when membership changes (callback from gossip).
func (n *Node) onMembershipChanged(aliveNodes []ring.Node) {
	log.Printf("[%s] Membership changed: %d alive nodes", n.nodeID, len(aliveNodes))
	
	// Rebuild ring with alive nodes only
	n.ringMu.Lock()
	newRing := ring.NewRing(n.ring.GetVNodes())
	newRing.SetNodes(aliveNodes)
	n.ring = newRing
	n.ringMu.Unlock()
	
	log.Printf("[%s] Ring updated with %d nodes", n.nodeID, len(aliveNodes))
}

// probeFn performs a ping probe for failure detection.
func (n *Node) probeFn(ctx context.Context, addr string) error {
	client, err := n.clientMgr.GetMembershipClient(addr)
	if err != nil {
		return err
	}
	
	req := &kvstorepb.PingRequest{
		FromId:      n.nodeID,
		TimestampMs: uint64(time.Now().UnixMilli()),
	}
	
	_, err = client.Ping(ctx, req)
	return err
}

// gossipFn sends gossip to propagate membership.
func (n *Node) gossipFn(ctx context.Context, addr string, members []*gossip.Member) error {
	client, err := n.clientMgr.GetMembershipClient(addr)
	if err != nil {
		return err
	}
	
	// Convert members to proto
	protoMembers := make([]*kvstorepb.Member, 0, len(members))
	for _, m := range members {
		protoMembers = append(protoMembers, &kvstorepb.Member{
			Id:             m.ID,
			Addr:           m.Addr,
			Status:         m.Status.ToProto(),
			Incarnation:    m.Incarnation,
			LastSeenUnixMs: uint64(m.LastSeen.UnixMilli()),
		})
	}
	
	req := &kvstorepb.GossipRequest{
		FromId:     n.nodeID,
		Membership: protoMembers,
	}
	
	_, err = client.Gossip(ctx, req)
	return err
}

