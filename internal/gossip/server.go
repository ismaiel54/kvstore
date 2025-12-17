package gossip

import (
	"context"
	"log"
	"time"

	kvstorepb "kvstore/internal/gen/api"
	"kvstore/internal/ring"
)

// Server implements the Membership gRPC service.
type Server struct {
	kvstorepb.UnimplementedMembershipServer
	membership        *Membership
	ring              *ring.Ring
	ringGetter        func() *ring.Ring // Thread-safe ring getter
	replicationFactor int
	startTime         time.Time
}

// NewServer creates a new membership server.
func NewServer(membership *Membership, r *ring.Ring, ringGetter func() *ring.Ring, rf int) *Server {
	return &Server{
		membership:        membership,
		ring:              r,
		ringGetter:        ringGetter,
		replicationFactor: rf,
		startTime:         time.Now(),
	}
}

// Ping handles ping requests for failure detection.
func (s *Server) Ping(ctx context.Context, req *kvstorepb.PingRequest) (*kvstorepb.PingResponse, error) {
	// Mark sender as alive
	s.membership.MarkAlive(req.FromId)

	// Optionally apply piggybacked membership
	if len(req.Membership) > 0 {
		members := protoToMembers(req.Membership)
		s.membership.ApplyGossip(members)
	}

	return &kvstorepb.PingResponse{
		ResponderId: s.membership.localID,
		TimestampMs: uint64(time.Now().UnixMilli()),
		Membership:  membersToProto(s.membership.Snapshot()),
	}, nil
}

// Gossip handles gossip requests for membership propagation.
func (s *Server) Gossip(ctx context.Context, req *kvstorepb.GossipRequest) (*kvstorepb.GossipResponse, error) {
	log.Printf("[%s] Received gossip from %s with %d members", s.membership.localID, req.FromId, len(req.Membership))

	// Apply received membership
	members := protoToMembers(req.Membership)
	s.membership.ApplyGossip(members)

	// Return our membership snapshot
	return &kvstorepb.GossipResponse{
		ResponderId: s.membership.localID,
		Membership:  membersToProto(s.membership.Snapshot()),
	}, nil
}

// GetMembership returns current membership state (debug endpoint).
func (s *Server) GetMembership(ctx context.Context, req *kvstorepb.GetMembershipRequest) (*kvstorepb.GetMembershipResponse, error) {
	members := s.membership.GetMembership()
	return &kvstorepb.GetMembershipResponse{
		Members:     membersToProto(members),
		LocalNodeId: s.membership.localID,
	}, nil
}

// GetRing returns ring information for a key (debug endpoint).
func (s *Server) GetRing(ctx context.Context, req *kvstorepb.GetRingRequest) (*kvstorepb.GetRingResponse, error) {
	rng := s.ringGetter()

	aliveNodes := s.membership.AliveNodes()

	if req.Key == "" {
		// Return general ring info
		return &kvstorepb.GetRingResponse{
			AliveMembers:      int32(len(aliveNodes)),
			ReplicationFactor: int32(s.replicationFactor),
		}, nil
	}

	// Get owner and replicas for the key
	owner, exists := rng.ResponsibleNode(req.Key)
	if !exists {
		return &kvstorepb.GetRingResponse{
			AliveMembers:      int32(len(aliveNodes)),
			ReplicationFactor: int32(s.replicationFactor),
		}, nil
	}

	replicas := rng.PreferenceList(req.Key, s.replicationFactor)

	replicaIDs := make([]string, 0, len(replicas))
	replicaAddrs := make([]string, 0, len(replicas))
	for _, r := range replicas {
		replicaIDs = append(replicaIDs, r.ID)
		replicaAddrs = append(replicaAddrs, r.Addr)
	}

	return &kvstorepb.GetRingResponse{
		OwnerId:           owner.ID,
		OwnerAddr:         owner.Addr,
		ReplicaIds:        replicaIDs,
		ReplicaAddrs:      replicaAddrs,
		AliveMembers:      int32(len(aliveNodes)),
		ReplicationFactor: int32(s.replicationFactor),
	}, nil
}

// Health returns health status (operability endpoint).
func (s *Server) Health(ctx context.Context, req *kvstorepb.HealthRequest) (*kvstorepb.HealthResponse, error) {
	aliveNodes := s.membership.AliveNodes()
	status := kvstorepb.HealthResponse_OK

	// Check if we have enough nodes for quorum
	if len(aliveNodes) < 2 {
		status = kvstorepb.HealthResponse_DEGRADED
	}

	uptime := uint64(time.Since(s.startTime).Seconds())

	return &kvstorepb.HealthResponse{
		Status:        status,
		NodeId:        s.membership.localID,
		UptimeSeconds: uptime,
		Message:       "operational",
	}, nil
}

// protoToMembers converts protobuf members to internal Member slice.
func protoToMembers(protoMembers []*kvstorepb.Member) []*Member {
	members := make([]*Member, 0, len(protoMembers))
	for _, pm := range protoMembers {
		members = append(members, &Member{
			ID:          pm.Id,
			Addr:        pm.Addr,
			Status:      FromProto(pm.Status),
			Incarnation: pm.Incarnation,
			LastSeen:    time.UnixMilli(int64(pm.LastSeenUnixMs)),
		})
	}
	return members
}

// membersToProto converts internal Member slice to protobuf.
func membersToProto(members []*Member) []*kvstorepb.Member {
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
	return protoMembers
}
