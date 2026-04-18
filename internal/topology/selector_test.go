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

	for i := 0; i < 100; i++ {
		selected := s.Select(nodes)
		require.NotNil(t, selected)
		assert.NotEqual(t, "self", selected.ID)
	}
}

func TestSelectorRoundRobinCoversAllPeers(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := makeNodes()

	// One full cycle should return each peer exactly once
	seen := make(map[string]int)
	peers := len(nodes) - 1 // exclude self
	for i := 0; i < peers; i++ {
		selected := s.Select(nodes)
		require.NotNil(t, selected)
		seen[selected.ID]++
	}

	assert.Len(t, seen, peers, "should visit every peer exactly once per cycle")
	for id, count := range seen {
		assert.Equal(t, 1, count, "peer %s should appear exactly once", id)
	}
}

func TestSelectorRoundRobinReshufflesAfterCycle(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := makeNodes()
	peers := len(nodes) - 1

	// Complete first cycle
	first := make([]string, peers)
	for i := 0; i < peers; i++ {
		first[i] = s.Select(nodes).ID
	}

	// Complete second cycle — should cover the same peers (possibly different order)
	second := make(map[string]bool)
	for i := 0; i < peers; i++ {
		second[s.Select(nodes).ID] = true
	}

	assert.Len(t, second, peers, "second cycle should also cover all peers")
}

func TestSelectorZoneBiasInOrdering(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.9)
	nodes := makeNodes()
	peers := len(nodes) - 1

	// With 0.9 local weight, local peers should tend to appear earlier.
	// Run many cycles and check the average position of local peers.
	cycles := 1000
	localPosSum := 0
	localCount := 0

	for c := 0; c < cycles; c++ {
		for i := 0; i < peers; i++ {
			sel := s.Select(nodes)
			if sel.Zone == "zone-a" {
				localPosSum += i
				localCount++
			}
		}
	}

	avgLocalPos := float64(localPosSum) / float64(localCount)
	avgOverall := float64(peers-1) / 2.0 // 1.5 for 4 peers

	t.Logf("Average local peer position: %.2f (overall midpoint: %.1f)", avgLocalPos, avgOverall)
	assert.Less(t, avgLocalPos, avgOverall,
		"local peers should appear earlier than average due to zone bias")
}

func TestSelectorNoRemotePeers(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)
	nodes := []*membership.Node{
		{ID: "self", Zone: "zone-a"},
		{ID: "local-1", Zone: "zone-a"},
	}

	for i := 0; i < 10; i++ {
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

func TestSelectorAdaptsToMembershipChanges(t *testing.T) {
	s := topology.NewSelector("self", "zone-a", 0.7)

	nodes3 := []*membership.Node{
		{ID: "self", Zone: "zone-a"},
		{ID: "a", Zone: "zone-a"},
		{ID: "b", Zone: "zone-b"},
	}

	// First call builds queue with 2 peers
	sel := s.Select(nodes3)
	require.NotNil(t, sel)

	// Add a new node — queue should rebuild
	nodes4 := append(nodes3, &membership.Node{ID: "c", Zone: "zone-a"})
	seen := make(map[string]bool)
	for i := 0; i < 3; i++ {
		sel = s.Select(nodes4)
		require.NotNil(t, sel)
		seen[sel.ID] = true
	}
	assert.True(t, seen["c"], "new node 'c' should appear after membership change")
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
