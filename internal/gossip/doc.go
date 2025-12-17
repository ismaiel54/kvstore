// Package gossip implements a simplified SWIM-style membership protocol
// for dynamic cluster membership and failure detection.
//
// Limitations (learning-grade implementation):
// - No data migration/rebalancing during membership changes
// - Partial availability possible during transitions
// - No anti-entropy beyond gossip
// - Suspect nodes excluded from ring (Alive only)
package gossip
