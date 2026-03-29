package membership

import (
	"sync"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
)

// Node is a versioned view of one member's identity, reachability, and placement.
type Node struct {
	ID      string
	Addr    string
	Zone    string
	State   pb.NodeState
	Version uint64
}

// List holds the local membership table and broadcasts state transitions to subscribers.
type List struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	self  string

	subscribers []chan StateChange
}

// StateChange describes a node's previous and new lifecycle state after an update.
type StateChange struct {
	NodeID   string
	OldState pb.NodeState
	NewState pb.NodeState
	Version  uint64
}

// NewList initializes a membership list whose sole entry is this process as alive.
func NewList(selfID, selfAddr, zone string) *List {
	l := &List{
		nodes: make(map[string]*Node),
		self:  selfID,
	}
	l.nodes[selfID] = &Node{
		ID:      selfID,
		Addr:    selfAddr,
		Zone:    zone,
		State:   pb.NodeState_NODE_STATE_ALIVE,
		Version: 1,
	}
	return l
}

// Subscribe returns a buffered channel of membership changes; slow readers may drop events.
func (l *List) Subscribe() <-chan StateChange {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan StateChange, 64)
	l.subscribers = append(l.subscribers, ch)
	return ch
}

func (l *List) notify(change StateChange) {
	for _, ch := range l.subscribers {
		select {
		case ch <- change:
		default:
		}
	}
}

// ApplyUpdate inserts or merges an update when its version is newer than the stored one.
func (l *List) ApplyUpdate(update *pb.StateUpdate) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	existing, ok := l.nodes[update.NodeId]
	if !ok {
		l.nodes[update.NodeId] = &Node{
			ID:      update.NodeId,
			Addr:    update.Addr,
			Zone:    update.Zone,
			State:   update.State,
			Version: update.Version,
		}
		l.notify(StateChange{
			NodeID:   update.NodeId,
			NewState: update.State,
			Version:  update.Version,
		})
		return true
	}

	if update.Version <= existing.Version {
		return false
	}

	oldState := existing.State
	existing.State = update.State
	existing.Version = update.Version
	if update.Addr != "" {
		existing.Addr = update.Addr
	}
	if update.Zone != "" {
		existing.Zone = update.Zone
	}

	if oldState != update.State {
		l.notify(StateChange{
			NodeID:   update.NodeId,
			OldState: oldState,
			NewState: update.State,
			Version:  update.Version,
		})
	}

	return true
}

// SetState updates a node's state in place, bumps its version, and notifies if it changed.
func (l *List) SetState(nodeID string, state pb.NodeState) {
	l.mu.Lock()
	defer l.mu.Unlock()

	n, ok := l.nodes[nodeID]
	if !ok {
		return
	}

	oldState := n.State
	n.State = state
	n.Version++

	if oldState != state {
		l.notify(StateChange{
			NodeID:   nodeID,
			OldState: oldState,
			NewState: state,
			Version:  n.Version,
		})
	}
}

// Get returns a copy of the node if present.
func (l *List) Get(nodeID string) (*Node, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	n, ok := l.nodes[nodeID]
	if !ok {
		return nil, false
	}
	cp := *n
	return &cp, true
}

// GetSelf returns a copy of the local member node, or nil if missing.
func (l *List) GetSelf() *Node {
	n, _ := l.Get(l.self)
	return n
}

// Remove deletes a node from the table without notifying subscribers.
func (l *List) Remove(nodeID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.nodes, nodeID)
}

// AliveMembers returns copies of nodes in ALIVE or SUSPECT state.
func (l *List) AliveMembers() []*Node {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*Node
	for _, n := range l.nodes {
		if n.State == pb.NodeState_NODE_STATE_ALIVE || n.State == pb.NodeState_NODE_STATE_SUSPECT {
			cp := *n
			result = append(result, &cp)
		}
	}
	return result
}

// AllNodes returns copies of every known node.
func (l *List) AllNodes() []*Node {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*Node, 0, len(l.nodes))
	for _, n := range l.nodes {
		cp := *n
		result = append(result, &cp)
	}
	return result
}

func (l *List) GetUpdates() []*pb.StateUpdate {
	l.mu.RLock()
	defer l.mu.RUnlock()

	updates := make([]*pb.StateUpdate, 0, len(l.nodes))
	for _, n := range l.nodes {
		updates = append(updates, &pb.StateUpdate{
			NodeId:  n.ID,
			State:   n.State,
			Version: n.Version,
			Zone:    n.Zone,
			Addr:    n.Addr,
		})
	}
	return updates
}
