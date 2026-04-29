package topology

import (
	"math/rand/v2"
	"sync"

	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

// Selector picks gossip targets using round-robin ordering with zone-aware
// shuffling. Each full cycle through the member list is shuffled to avoid
// pathological ordering, but within a cycle every peer is probed exactly once
// before any peer is probed again. This guarantees failure detection within
// N × GossipInterval for an N-node cluster.
type Selector struct {
	mu          sync.Mutex
	localWeight float64
	selfID      string
	selfZone    string

	// Round-robin probe queue: shuffled list of peer IDs, walked sequentially.
	probeQueue []*membership.Node
	probeIdx   int
}

// NewSelector creates a Selector that biases localWeight toward same-zone peers.
func NewSelector(selfID, selfZone string, localWeight float64) *Selector {
	return &Selector{
		localWeight: localWeight,
		selfID:      selfID,
		selfZone:    selfZone,
	}
}

// Select returns the next peer in the round-robin probe order. When the queue
// is exhausted or the member set changes, it rebuilds and reshuffles the queue
// with zone-aware ordering: local-zone peers are placed earlier in the queue
// proportionally to localWeight.
func (s *Selector) Select(members []*membership.Node) *membership.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	peers := s.filterSelf(members)
	if len(peers) == 0 {
		return nil
	}

	// Rebuild the queue if exhausted or member set changed
	if s.probeIdx >= len(s.probeQueue) || s.memberSetChanged(peers) {
		s.rebuildQueue(peers)
	}

	target := s.probeQueue[s.probeIdx]
	s.probeIdx++
	return target
}

// SelectN picks up to n distinct peers, mixing zones according to weights.
func (s *Selector) SelectN(members []*membership.Node, n int) []*membership.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	candidates := s.filterSelf(members)
	if len(candidates) <= n {
		return candidates
	}

	rand.Shuffle(len(candidates), func(i, j int) { //nolint:gosec // not security-sensitive
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	return candidates[:n]
}

func (s *Selector) filterSelf(members []*membership.Node) []*membership.Node {
	out := make([]*membership.Node, 0, len(members))
	for _, m := range members {
		if m.ID != s.selfID {
			out = append(out, m)
		}
	}
	return out
}

// memberSetChanged detects if the live peer set differs from the current queue.
func (s *Selector) memberSetChanged(peers []*membership.Node) bool {
	if len(peers) != len(s.probeQueue) {
		return true
	}
	current := make(map[string]struct{}, len(s.probeQueue))
	for _, n := range s.probeQueue {
		current[n.ID] = struct{}{}
	}
	for _, p := range peers {
		if _, ok := current[p.ID]; !ok {
			return true
		}
	}
	return false
}

// rebuildQueue creates a new shuffled probe order. Zone-aware bias is applied
// by interleaving local and remote peers: pick from local with probability
// localWeight, otherwise pick from remote, until all peers are placed.
func (s *Selector) rebuildQueue(peers []*membership.Node) {
	var local, remote []*membership.Node
	for _, p := range peers {
		if p.Zone == s.selfZone {
			local = append(local, p)
		} else {
			remote = append(remote, p)
		}
	}

	rand.Shuffle(len(local), func(i, j int) { //nolint:gosec // not security-sensitive
		local[i], local[j] = local[j], local[i]
	})
	rand.Shuffle(len(remote), func(i, j int) { //nolint:gosec // not security-sensitive
		remote[i], remote[j] = remote[j], remote[i]
	})

	queue := make([]*membership.Node, 0, len(peers))
	li, ri := 0, 0
	for li < len(local) || ri < len(remote) {
		pickLocal := false
		if li < len(local) && ri < len(remote) {
			pickLocal = rand.Float64() < s.localWeight //nolint:gosec // not security-sensitive
		} else if li < len(local) {
			pickLocal = true
		}

		if pickLocal {
			queue = append(queue, local[li])
			li++
		} else {
			queue = append(queue, remote[ri])
			ri++
		}
	}

	s.probeQueue = queue
	s.probeIdx = 0
}
