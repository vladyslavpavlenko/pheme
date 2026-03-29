package topology

import (
	"math/rand/v2"
	"sync"

	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

// Selector picks gossip targets with zone-aware weighted probability.
type Selector struct {
	mu          sync.RWMutex
	localWeight float64
	selfID      string
	selfZone    string
}

// NewSelector creates a Selector that biases localWeight toward same-zone peers.
func NewSelector(selfID, selfZone string, localWeight float64) *Selector {
	return &Selector{
		localWeight: localWeight,
		selfID:      selfID,
		selfZone:    selfZone,
	}
}

// Select picks a peer using zone-aware weighted probability.
// Returns nil if no suitable peer is available.
func (s *Selector) Select(members []*membership.Node) *membership.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	var local, remote []*membership.Node
	for _, m := range members {
		if m.ID == s.selfID {
			continue
		}
		if m.Zone == s.selfZone {
			local = append(local, m)
		} else {
			remote = append(remote, m)
		}
	}

	if len(local) == 0 && len(remote) == 0 {
		return nil
	}

	if len(local) == 0 {
		return remote[rand.IntN(len(remote))] //nolint:gosec // not security-sensitive
	}
	if len(remote) == 0 {
		return local[rand.IntN(len(local))] //nolint:gosec // not security-sensitive
	}

	if rand.Float64() < s.localWeight { //nolint:gosec // not security-sensitive
		return local[rand.IntN(len(local))] //nolint:gosec // not security-sensitive
	}
	return remote[rand.IntN(len(remote))] //nolint:gosec // not security-sensitive
}

// SelectN picks up to n distinct peers, mixing zones according to weights.
func (s *Selector) SelectN(members []*membership.Node, n int) []*membership.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	var candidates []*membership.Node
	for _, m := range members {
		if m.ID != s.selfID {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) <= n {
		return candidates
	}

	rand.Shuffle(len(candidates), func(i, j int) { //nolint:gosec // not security-sensitive
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	return candidates[:n]
}
