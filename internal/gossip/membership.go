package gossip

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"kvstore/internal/ring"
	kvstorepb "kvstore/internal/gen/api"
)

// MemberStatus represents the state of a cluster member.
type MemberStatus int

const (
	Alive MemberStatus = iota
	Suspect
	Dead
)

// String returns the string representation of MemberStatus.
func (s MemberStatus) String() string {
	switch s {
	case Alive:
		return "ALIVE"
	case Suspect:
		return "SUSPECT"
	case Dead:
		return "DEAD"
	default:
		return "UNKNOWN"
	}
}

// ToProto converts MemberStatus to protobuf enum.
func (s MemberStatus) ToProto() kvstorepb.MemberStatus {
	switch s {
	case Alive:
		return kvstorepb.MemberStatus_ALIVE
	case Suspect:
		return kvstorepb.MemberStatus_SUSPECT
	case Dead:
		return kvstorepb.MemberStatus_DEAD
	default:
		return kvstorepb.MemberStatus_ALIVE
	}
}

// FromProto converts protobuf enum to MemberStatus.
func FromProto(s kvstorepb.MemberStatus) MemberStatus {
	switch s {
	case kvstorepb.MemberStatus_ALIVE:
		return Alive
	case kvstorepb.MemberStatus_SUSPECT:
		return Suspect
	case kvstorepb.MemberStatus_DEAD:
		return Dead
	default:
		return Alive
	}
}

// Member represents a cluster member.
type Member struct {
	ID          string
	Addr        string
	Status      MemberStatus
	Incarnation uint64
	LastSeen    time.Time
}

// Membership manages cluster membership with gossip-based failure detection.
type Membership struct {
	mu          sync.RWMutex
	localID     string
	localAddr   string
	members     map[string]*Member // id -> Member
	incarnation map[string]uint64  // id -> incarnation (for local tracking)

	// Configuration
	probeInterval  time.Duration
	suspectTimeout time.Duration
	deadTimeout    time.Duration

	// Callbacks
	onMembershipChanged func([]ring.Node)

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewMembership creates a new membership manager.
func NewMembership(localID, localAddr string, probeInterval, suspectTimeout, deadTimeout time.Duration) *Membership {
	if probeInterval <= 0 {
		probeInterval = 1 * time.Second
	}
	if suspectTimeout <= 0 {
		suspectTimeout = 3 * time.Second
	}
	if deadTimeout <= 0 {
		deadTimeout = 10 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Membership{
		localID:       localID,
		localAddr:     localAddr,
		members:       make(map[string]*Member),
		incarnation:   make(map[string]uint64),
		probeInterval: probeInterval,
		suspectTimeout: suspectTimeout,
		deadTimeout:   deadTimeout,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Add self as Alive
	m.members[localID] = &Member{
		ID:          localID,
		Addr:        localAddr,
		Status:      Alive,
		Incarnation: 1,
		LastSeen:    time.Now(),
	}
	m.incarnation[localID] = 1

	return m
}

// SetOnMembershipChanged sets a callback that's invoked when membership changes.
func (m *Membership) SetOnMembershipChanged(callback func([]ring.Node)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMembershipChanged = callback
}

// Start starts the membership protocol (probes and gossip).
func (m *Membership) Start(probeFn func(ctx context.Context, addr string) error, gossipFn func(ctx context.Context, addr string, members []*Member) error) {
	m.wg.Add(2)

	// Probe loop
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.probeInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.probe(probeFn)
			}
		}
	}()

	// Gossip loop
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.probeInterval * 2) // Gossip less frequently
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.gossip(gossipFn)
			}
		}
	}()

	// Timeout checker
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				m.checkTimeouts()
			}
		}
	}()
}

// Stop stops the membership protocol.
func (m *Membership) Stop() {
	m.cancel()
	m.wg.Wait()
}

// probe performs a failure detection probe to a random peer.
func (m *Membership) probe(probeFn func(ctx context.Context, addr string) error) {
	m.mu.RLock()
	alive := m.getAliveMembers()
	m.mu.RUnlock()

	if len(alive) == 0 {
		return
	}

	// Pick random peer (excluding self)
	candidates := make([]*Member, 0)
	for _, member := range alive {
		if member.ID != m.localID {
			candidates = append(candidates, member)
		}
	}

	if len(candidates) == 0 {
		return
	}

	target := candidates[rand.Intn(len(candidates))]

	// Probe with timeout
	ctx, cancel := context.WithTimeout(m.ctx, m.probeInterval)
	defer cancel()

	err := probeFn(ctx, target.Addr)
	m.mu.Lock()
	defer m.mu.Unlock()

	if err == nil {
		// Success - mark as Alive
		if member, exists := m.members[target.ID]; exists {
			member.Status = Alive
			member.LastSeen = time.Now()
			if member.Incarnation < m.incarnation[target.ID] {
				member.Incarnation = m.incarnation[target.ID]
			}
		}
		m.notifyMembershipChanged()
	} else {
		// Failure - mark as Suspect
		if member, exists := m.members[target.ID]; exists && member.Status == Alive {
			m.incarnation[target.ID]++
			member.Status = Suspect
			member.Incarnation = m.incarnation[target.ID]
			member.LastSeen = time.Now()
			log.Printf("[%s] Marked %s as SUSPECT (probe failed)", m.localID, target.ID)
			m.notifyMembershipChanged()
		}
	}
}

// gossip propagates membership information to a random peer.
func (m *Membership) gossip(gossipFn func(ctx context.Context, addr string, members []*Member) error) {
	m.mu.RLock()
	snapshot := m.Snapshot()
	allMembers := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		allMembers = append(allMembers, member)
	}
	m.mu.RUnlock()

	if len(snapshot) == 0 {
		return
	}

	// Pick random peer
	target := snapshot[rand.Intn(len(snapshot))]
	if target.ID == m.localID {
		return
	}

	ctx, cancel := context.WithTimeout(m.ctx, m.probeInterval)
	defer cancel()

	_ = gossipFn(ctx, target.Addr, allMembers) // Best effort
}

// checkTimeouts checks for suspect/dead timeouts.
func (m *Membership) checkTimeouts() {
	now := time.Now()
	changed := false

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, member := range m.members {
		if id == m.localID {
			continue
		}

		elapsed := now.Sub(member.LastSeen)

		if member.Status == Suspect && elapsed > m.suspectTimeout {
			// Suspect -> Dead
			m.incarnation[id]++
			member.Status = Dead
			member.Incarnation = m.incarnation[id]
			log.Printf("[%s] Marked %s as DEAD (suspect timeout)", m.localID, id)
			changed = true
		} else if member.Status == Dead && elapsed > m.deadTimeout {
			// Remove dead nodes after deadTimeout (optional cleanup)
			// For now, keep them but don't include in ring
		}
	}

	if changed {
		m.notifyMembershipChanged()
	}
}

// ApplyGossip merges received membership information.
func (m *Membership) ApplyGossip(remoteMembers []*Member) {
	m.mu.Lock()
	defer m.mu.Unlock()

	changed := false
	for _, remote := range remoteMembers {
		if remote.ID == m.localID {
			continue // Ignore self
		}

		local, exists := m.members[remote.ID]

		if !exists {
			// New member
			m.members[remote.ID] = &Member{
				ID:          remote.ID,
				Addr:        remote.Addr,
				Status:      remote.Status,
				Incarnation: remote.Incarnation,
				LastSeen:    time.Now(),
			}
			m.incarnation[remote.ID] = remote.Incarnation
			changed = true
			log.Printf("[%s] Discovered new member: %s (%s)", m.localID, remote.ID, remote.Status)
		} else {
			// Merge: higher incarnation wins
			if remote.Incarnation > local.Incarnation {
				local.Status = remote.Status
				local.Incarnation = remote.Incarnation
				local.LastSeen = time.Now()
				m.incarnation[remote.ID] = remote.Incarnation
				changed = true
				log.Printf("[%s] Updated %s: incarnation=%d status=%s", m.localID, remote.ID, remote.Incarnation, remote.Status)
			} else if remote.Incarnation == local.Incarnation {
				// Same incarnation: prefer Alive > Suspect > Dead
				if shouldUpdateStatus(local.Status, remote.Status) {
					local.Status = remote.Status
					local.LastSeen = time.Now()
					changed = true
				}
			}
			// If remote.Incarnation < local.Incarnation, ignore (local is newer)
		}
	}

	if changed {
		m.notifyMembershipChanged()
	}
}

// shouldUpdateStatus returns true if remote status should replace local status
// when incarnations are equal. Prefers: Alive > Suspect > Dead
func shouldUpdateStatus(local, remote MemberStatus) bool {
	if remote == Alive && local != Alive {
		return true
	}
	if remote == Suspect && local == Dead {
		return true
	}
	return false
}

// MarkAlive marks a member as alive (called on successful ping).
func (m *Membership) MarkAlive(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if member, exists := m.members[id]; exists {
		if member.Status != Alive {
			member.Status = Alive
			member.LastSeen = time.Now()
			log.Printf("[%s] Marked %s as ALIVE", m.localID, id)
			m.notifyMembershipChanged()
		} else {
			member.LastSeen = time.Now()
		}
	}
}

// Snapshot returns a snapshot of all members.
func (m *Membership) Snapshot() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := make([]*Member, 0, len(m.members))
	for _, member := range m.members {
		snapshot = append(snapshot, &Member{
			ID:          member.ID,
			Addr:        member.Addr,
			Status:      member.Status,
			Incarnation: member.Incarnation,
			LastSeen:    member.LastSeen,
		})
	}
	return snapshot
}

// AliveNodes returns only Alive members as ring.Node slice.
func (m *Membership) AliveNodes() []ring.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nodes := make([]ring.Node, 0)
	for _, member := range m.members {
		if member.Status == Alive {
			nodes = append(nodes, ring.Node{
				ID:   member.ID,
				Addr: member.Addr,
			})
		}
	}
	return nodes
}

// GetMembership returns current membership state (for debug endpoint).
func (m *Membership) GetMembership() []*Member {
	return m.Snapshot()
}

// AddSeedMembers adds seed members for initial discovery.
func (m *Membership) AddSeedMembers(seeds []ring.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, seed := range seeds {
		if seed.ID == m.localID {
			continue
		}
		if _, exists := m.members[seed.ID]; !exists {
			m.members[seed.ID] = &Member{
				ID:          seed.ID,
				Addr:        seed.Addr,
				Status:      Alive, // Assume alive initially
				Incarnation: 1,
				LastSeen:    time.Now(),
			}
			m.incarnation[seed.ID] = 1
		}
	}
	m.notifyMembershipChanged()
}

// getAliveMembers returns alive members (must be called with lock held).
func (m *Membership) getAliveMembers() []*Member {
	alive := make([]*Member, 0)
	for _, member := range m.members {
		if member.Status == Alive {
			alive = append(alive, member)
		}
	}
	return alive
}

// notifyMembershipChanged invokes the callback if set.
func (m *Membership) notifyMembershipChanged() {
	if m.onMembershipChanged != nil {
		alive := m.AliveNodes()
		go m.onMembershipChanged(alive) // Async to avoid blocking
	}
}

