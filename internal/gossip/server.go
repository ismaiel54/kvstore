package gossip

import (
	"context"
	"log"
	"time"

	kvstorepb "kvstore/internal/gen/api"
)

// Server implements the Membership gRPC service.
type Server struct {
	kvstorepb.UnimplementedMembershipServer
	membership *Membership
}

// NewServer creates a new membership server.
func NewServer(membership *Membership) *Server {
	return &Server{
		membership: membership,
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
		ResponderId:  s.membership.localID,
		TimestampMs:  uint64(time.Now().UnixMilli()),
		Membership:   membersToProto(s.membership.Snapshot()),
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
		Members:      membersToProto(members),
		LocalNodeId:  s.membership.localID,
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
			Id:              m.ID,
			Addr:            m.Addr,
			Status:          m.Status.ToProto(),
			Incarnation:     m.Incarnation,
			LastSeenUnixMs:  uint64(m.LastSeen.UnixMilli()),
		})
	}
	return protoMembers
}

