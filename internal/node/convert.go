package node

import (
	"kvstore/internal/clock"
	kvstorepb "kvstore/internal/gen/api"
)

// protoToVectorClock converts a protobuf VectorClock to internal clock.VectorClock.
func protoToVectorClock(pb *kvstorepb.VectorClock) clock.VectorClock {
	if pb == nil {
		return nil
	}
	vc := clock.New()
	for _, entry := range pb.Entries {
		vc.Set(entry.NodeId, entry.Counter)
	}
	return vc
}

// vectorClockToProto converts an internal clock.VectorClock to protobuf VectorClock.
func vectorClockToProto(vc clock.VectorClock) *kvstorepb.VectorClock {
	if vc == nil || len(vc) == 0 {
		return &kvstorepb.VectorClock{Entries: []*kvstorepb.VectorClockEntry{}}
	}
	pb := &kvstorepb.VectorClock{
		Entries: make([]*kvstorepb.VectorClockEntry, 0, len(vc)),
	}
	for nodeID, counter := range vc {
		pb.Entries = append(pb.Entries, &kvstorepb.VectorClockEntry{
			NodeId:  nodeID,
			Counter: counter,
		})
	}
	return pb
}

