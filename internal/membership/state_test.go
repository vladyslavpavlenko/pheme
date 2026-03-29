package membership_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

func TestNewList(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	self := l.GetSelf()
	require.NotNil(t, self)
	assert.Equal(t, "node-1", self.ID)
	assert.Equal(t, pb.NodeState_NODE_STATE_ALIVE, self.State)
}

func TestApplyUpdateNewNode(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	applied := l.ApplyUpdate(&pb.StateUpdate{
		NodeId:  "node-2",
		State:   pb.NodeState_NODE_STATE_ALIVE,
		Version: 1,
		Zone:    "zone-b",
		Addr:    "127.0.0.1:7947",
	})

	require.True(t, applied)

	n, ok := l.Get("node-2")
	require.True(t, ok)
	assert.Equal(t, "zone-b", n.Zone)
}

func TestApplyUpdateOlderVersion(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 5})

	applied := l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_DEAD, Version: 3})
	assert.False(t, applied, "older version should not be applied")

	n, _ := l.Get("node-2")
	assert.Equal(t, pb.NodeState_NODE_STATE_ALIVE, n.State)
}

func TestApplyUpdateNewerVersion(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 1})
	applied := l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_SUSPECT, Version: 2})

	require.True(t, applied)

	n, _ := l.Get("node-2")
	assert.Equal(t, pb.NodeState_NODE_STATE_SUSPECT, n.State)
}

func TestSetState(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	l.SetState("node-1", pb.NodeState_NODE_STATE_SUSPECT)

	self := l.GetSelf()
	assert.Equal(t, pb.NodeState_NODE_STATE_SUSPECT, self.State)
}

func TestAliveMembers(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 1})
	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-3", State: pb.NodeState_NODE_STATE_DEAD, Version: 1})
	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-4", State: pb.NodeState_NODE_STATE_SUSPECT, Version: 1})

	assert.Len(t, l.AliveMembers(), 3)
}

func TestSubscribe(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	ch := l.Subscribe()

	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 1})

	select {
	case change := <-ch:
		assert.Equal(t, "node-2", change.NodeID)
	default:
		t.Fatal("expected a state change notification")
	}
}

func TestGetUpdates(t *testing.T) {
	l := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	l.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 1, Zone: "zone-b"})

	assert.Len(t, l.GetUpdates(), 2)
}
