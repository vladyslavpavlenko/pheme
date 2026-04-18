package membership_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

func makeUpdate(nodeID string, version uint64) *pb.StateUpdate {
	return &pb.StateUpdate{
		NodeId:  nodeID,
		State:   pb.NodeState_NODE_STATE_ALIVE,
		Version: version,
		Zone:    "z1",
		Addr:    "127.0.0.1:9000",
	}
}

func TestBroadcastQueue_BasicRetransmit(t *testing.T) {
	q := membership.NewBroadcastQueue(2) // mult=2
	q.Enqueue(makeUpdate("a", 1), 4)     // limit = 2 * ceil(log2(5)) = 6

	var gotCount int
	for range 20 {
		got := q.GetBroadcasts(2000)
		if len(got) == 0 {
			break
		}
		gotCount++
	}
	assert.Equal(t, 6, gotCount)
}

func TestBroadcastQueue_ReplaceDuplicateNode(t *testing.T) {
	q := membership.NewBroadcastQueue(2)
	q.Enqueue(makeUpdate("a", 1), 1)
	q.Enqueue(makeUpdate("a", 2), 1)

	assert.Equal(t, 1, q.Len())

	got := q.GetBroadcasts(2000)
	require.Len(t, got, 1)
	assert.Equal(t, uint64(2), got[0].Version)
}

func TestBroadcastQueue_RespectsByteLimit(t *testing.T) {
	q := membership.NewBroadcastQueue(3)
	q.Enqueue(makeUpdate("a", 1), 4)
	q.Enqueue(makeUpdate("b", 1), 4)
	q.Enqueue(makeUpdate("c", 1), 4)

	// Each update is ~34 bytes. Allow only 40 bytes → should get 1 entry.
	got := q.GetBroadcasts(40)
	assert.Equal(t, 1, len(got))
}

func TestBroadcastQueue_EmptyOnSteadyState(t *testing.T) {
	q := membership.NewBroadcastQueue(1)
	q.Enqueue(makeUpdate("a", 1), 1)

	limit := q.RetransmitLimit(1)
	for range limit {
		q.GetBroadcasts(2000)
	}

	got := q.GetBroadcasts(2000)
	assert.Empty(t, got)
}

func TestBroadcastQueue_PrioritizesHighRetransmit(t *testing.T) {
	q := membership.NewBroadcastQueue(2)
	q.Enqueue(makeUpdate("old", 1), 4) // gets limit 6

	// Drain "old" partially (3 of 6 transmits)
	for range 3 {
		q.GetBroadcasts(2000)
	}

	q.Enqueue(makeUpdate("new", 1), 4) // fresh, gets full limit 6

	got := q.GetBroadcasts(2000)
	require.Len(t, got, 2)
	assert.Equal(t, "new", got[0].NodeId) // higher remaining count = first
}
