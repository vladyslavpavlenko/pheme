package membership

import (
	"math"
	"sort"
	"sync"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
)

// broadcastEntry is a pending state update awaiting retransmission.
type broadcastEntry struct {
	update        *pb.StateUpdate
	transmitsLeft int
}

// BroadcastQueue holds recently-changed state updates and limits retransmission
// to RetransmitMult × log2(N+1) times, matching memberlist's approach.
// Once an entry exhausts its retransmit budget, it is dropped.
type BroadcastQueue struct {
	mu             sync.Mutex
	entries        []*broadcastEntry
	retransmitMult int
}

// NewBroadcastQueue creates a queue with the given retransmit multiplier.
func NewBroadcastQueue(retransmitMult int) *BroadcastQueue {
	return &BroadcastQueue{
		retransmitMult: retransmitMult,
	}
}

// RetransmitLimit computes the retransmit budget for the given cluster size.
func (q *BroadcastQueue) RetransmitLimit(nodeCount int) int {
	if nodeCount < 1 {
		nodeCount = 1
	}
	limit := q.retransmitMult * int(math.Ceil(math.Log2(float64(nodeCount)+1)))
	if limit < 1 {
		limit = 1
	}
	return limit
}

// Enqueue adds or replaces an update for the given node. nodeCount is the
// current cluster size, used for the retransmit limit. Callers must provide it
// to avoid re-entrant locking.
func (q *BroadcastQueue) Enqueue(u *pb.StateUpdate, nodeCount int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	limit := q.RetransmitLimit(nodeCount)

	for i, e := range q.entries {
		if e.update.NodeId == u.NodeId {
			q.entries[i] = &broadcastEntry{update: u, transmitsLeft: limit}
			return
		}
	}
	q.entries = append(q.entries, &broadcastEntry{update: u, transmitsLeft: limit})
}

// GetBroadcasts returns up to maxBytes worth of updates, ordered by most
// retransmits remaining (highest priority first). Each returned entry has its
// counter decremented; entries reaching zero are pruned.
func (q *BroadcastQueue) GetBroadcasts(maxBytes int) []*pb.StateUpdate {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.entries) == 0 {
		return nil
	}

	sort.Slice(q.entries, func(i, j int) bool {
		return q.entries[i].transmitsLeft > q.entries[j].transmitsLeft
	})

	var result []*pb.StateUpdate
	totalSize := 0

	for _, e := range q.entries {
		size := estimateUpdateSize(e.update)
		if totalSize+size > maxBytes {
			break
		}
		result = append(result, e.update)
		totalSize += size
		e.transmitsLeft--
	}

	// Prune exhausted entries
	alive := q.entries[:0]
	for _, e := range q.entries {
		if e.transmitsLeft > 0 {
			alive = append(alive, e)
		}
	}
	q.entries = alive

	return result
}

// Len returns the number of pending broadcasts.
func (q *BroadcastQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries)
}

func estimateUpdateSize(u *pb.StateUpdate) int {
	return 20 + len(u.NodeId) + len(u.Zone) + len(u.Addr)
}
