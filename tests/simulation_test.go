package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vladyslavpavlenko/pheme/tests/pkg/simulation"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
)

func TestSmallClusterConvergence(t *testing.T) {
	cc := simulation.ClusterConfig{
		NumNodes:       5,
		NumZones:       2,
		BasePort:       40000,
		GossipInterval: 100 * time.Millisecond,
	}

	cluster, err := simulation.NewCluster(cc)
	require.NoError(t, err)
	defer cluster.Shutdown()

	cluster.Start()

	dur, ok := cluster.WaitForFullMembership(10 * time.Second)
	require.True(t, ok, "cluster did not reach full membership within 10s")
	t.Logf("Full membership achieved in %v", dur)

	for _, n := range cluster.Nodes() {
		members := n.Members.AliveMembers()
		t.Logf("Node %s sees %d alive members", n.ID, len(members))
		assert.Equal(t, 5, len(members))
	}
}

func TestConvergenceAfterNodeDeath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping convergence test in short mode")
	}

	cc := simulation.ClusterConfig{
		NumNodes:       5,
		NumZones:       2,
		BasePort:       41000,
		GossipInterval: 100 * time.Millisecond,
	}

	cluster, err := simulation.NewCluster(cc)
	require.NoError(t, err)
	defer cluster.Shutdown()

	cluster.Start()

	dur, ok := cluster.WaitForFullMembership(10 * time.Second)
	require.True(t, ok, "cluster did not reach full membership within 10s")
	t.Logf("Full membership achieved in %v", dur)

	for _, n := range cluster.Nodes() {
		members := n.Members.AliveMembers()
		t.Logf("Node %s sees %d alive members before kill", n.ID, len(members))
	}

	target := cluster.Nodes()[4]
	t.Logf("Killing node %s", target.ID)
	cluster.KillNode(4)

	dur, converged := cluster.WaitForConvergence(
		target.ID, pb.NodeState_NODE_STATE_DEAD, 30*time.Second,
	)
	require.True(t, converged,
		"cluster should converge on dead state within 30s (took %v)", dur)
	t.Logf("Convergence achieved in %v", dur)
}
