package topology_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
	"github.com/vladyslavpavlenko/pheme/internal/topology"
)

func makeNodes() []*membership.Node {
	return []*membership.Node{
		{ID: "self", Addr: "127.0.0.1:1000", Zone: "zone-a", State: pb.NodeState_NODE_STATE_ALIVE},
		{ID: "local-1", Addr: "127.0.0.1:1001", Zone: "zone-a", State: pb.NodeState_NODE_STATE_ALIVE},
		{ID: "local-2", Addr: "127.0.0.1:1002", Zone: "zone-a", State: pb.NodeState_NODE_STATE_ALIVE},
		{ID: "remote-1", Addr: "127.0.0.1:2001", Zone: "zone-b", State: pb.NodeState_NODE_STATE_ALIVE},
		{ID: "remote-2", Addr: "127.0.0.1:2002", Zone: "zone-b", State: pb.NodeState_NODE_STATE_ALIVE},
	}
}

func TestSelectorNeverSelectsSelf(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := makeNodes()

	for i := 0; i < 1000; i++ {
		selected := s.Select(nodes)
		require.NotNil(t, selected)
		assert.NotEqual(t, "self", selected.ID)
	}
}

func TestSelectorZoneBias(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := makeNodes()

	localCount := 0
	total := 10000

	for i := 0; i < total; i++ {
		selected := s.Select(nodes)
		require.NotNil(t, selected, "selector should not return nil with available peers")
		if selected.Zone == "zone-a" {
			localCount++
		}
	}

	localRatio := float64(localCount) / float64(total)
	t.Logf("Local selection ratio: %.3f (expected ~0.7)", localRatio)

	assert.InDelta(t, 0.7, localRatio, 0.1, "local ratio should be ~0.7")
}

func TestSelectorNoRemotePeers(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := []*membership.Node{
		{ID: "self", Zone: "zone-a"},
		{ID: "local-1", Zone: "zone-a"},
	}

	for i := 0; i < 100; i++ {
		selected := s.Select(nodes)
		require.NotNil(t, selected)
		assert.Equal(t, "local-1", selected.ID)
	}
}

func TestSelectorNoPeers(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := []*membership.Node{{ID: "self", Zone: "zone-a"}}

	assert.Nil(t, s.Select(nodes))
}

func TestSelectN(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := makeNodes()

	selected := s.SelectN(nodes, 3)
	require.Len(t, selected, 3)

	for _, n := range selected {
		assert.NotEqual(t, "self", n.ID, "SelectN should not include self")
	}
}
