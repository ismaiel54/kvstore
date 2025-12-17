package ring

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
)

// Node represents a physical node in the cluster.
type Node struct {
	ID   string
	Addr string
}

// vnode represents a virtual node on the ring.
type vnode struct {
	hash   uint32
	nodeID string
}

// Ring implements consistent hashing with virtual nodes.
type Ring struct {
	mu          sync.RWMutex
	vnodesPerNode int
	vnodes      []vnode
	nodes       map[string]Node // nodeID -> Node
}

// NewRing creates a new consistent hashing ring.
func NewRing(vnodesPerNode int) *Ring {
	if vnodesPerNode <= 0 {
		vnodesPerNode = 128 // default
	}
	return &Ring{
		vnodesPerNode: vnodesPerNode,
		vnodes:        make([]vnode, 0),
		nodes:         make(map[string]Node),
	}
}

// SetNodes rebuilds the ring with the given nodes.
// This is deterministic: same nodes in same order produce same ring.
func (r *Ring) SetNodes(nodes []Node) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing
	r.nodes = make(map[string]Node)
	r.vnodes = make([]vnode, 0)

	// Add nodes
	for _, node := range nodes {
		r.nodes[node.ID] = node
		// Create virtual nodes for this physical node
		for i := 0; i < r.vnodesPerNode; i++ {
			vnodeID := fmt.Sprintf("%s-vnode-%d", node.ID, i)
			hash := r.hashString(vnodeID)
			r.vnodes = append(r.vnodes, vnode{
				hash:   hash,
				nodeID: node.ID,
			})
		}
	}

	// Sort vnodes by hash for binary search
	sort.Slice(r.vnodes, func(i, j int) bool {
		return r.vnodes[i].hash < r.vnodes[j].hash
	})
}

// AddNode adds a node to the ring.
func (r *Ring) AddNode(node Node) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[node.ID]; exists {
		return // already exists
	}

	r.nodes[node.ID] = node
	// Add virtual nodes
	for i := 0; i < r.vnodesPerNode; i++ {
		vnodeID := fmt.Sprintf("%s-vnode-%d", node.ID, i)
		hash := r.hashString(vnodeID)
		v := vnode{hash: hash, nodeID: node.ID}
		// Insert in sorted order
		idx := sort.Search(len(r.vnodes), func(i int) bool {
			return r.vnodes[i].hash >= hash
		})
		r.vnodes = append(r.vnodes[:idx], append([]vnode{v}, r.vnodes[idx:]...)...)
	}
}

// RemoveNode removes a node from the ring.
func (r *Ring) RemoveNode(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[nodeID]; !exists {
		return // doesn't exist
	}

	delete(r.nodes, nodeID)
	// Remove all vnodes for this node
	newVnodes := make([]vnode, 0, len(r.vnodes))
	for _, v := range r.vnodes {
		if v.nodeID != nodeID {
			newVnodes = append(newVnodes, v)
		}
	}
	r.vnodes = newVnodes
}

// ResponsibleNode returns the node responsible for the given key.
// Returns (Node, true) if found, (Node{}, false) if ring is empty.
func (r *Ring) ResponsibleNode(key string) (Node, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.vnodes) == 0 {
		return Node{}, false
	}

	keyHash := r.hashString(key)

	// Binary search for first vnode with hash >= keyHash
	idx := sort.Search(len(r.vnodes), func(i int) bool {
		return r.vnodes[i].hash >= keyHash
	})

	// Wrap around if keyHash is greater than all vnodes
	if idx >= len(r.vnodes) {
		idx = 0
	}

	nodeID := r.vnodes[idx].nodeID
	node, exists := r.nodes[nodeID]
	return node, exists
}

// PreferenceList returns the first k nodes in the preference list for the key.
// This is useful for replication in later phases.
func (r *Ring) PreferenceList(key string, k int) []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.vnodes) == 0 || k <= 0 {
		return []Node{}
	}

	keyHash := r.hashString(key)
	idx := sort.Search(len(r.vnodes), func(i int) bool {
		return r.vnodes[i].hash >= keyHash
	})

	if idx >= len(r.vnodes) {
		idx = 0
	}

	seen := make(map[string]bool)
	result := make([]Node, 0, k)

	// Start from the responsible node and walk forward
	for i := 0; i < len(r.vnodes) && len(result) < k; i++ {
		pos := (idx + i) % len(r.vnodes)
		nodeID := r.vnodes[pos].nodeID
		if !seen[nodeID] {
			seen[nodeID] = true
			if node, exists := r.nodes[nodeID]; exists {
				result = append(result, node)
			}
		}
	}

	return result
}

// GetNodes returns all nodes in the ring.
func (r *Ring) GetNodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	nodes := make([]Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// hashString computes a 32-bit FNV-1a hash of the string.
func (r *Ring) hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

