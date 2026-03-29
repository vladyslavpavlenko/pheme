package simulation

import (
	crand "crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/vladyslavpavlenko/pheme/internal/config"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/gossip"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

type NodeHandle struct {
	ID      string
	Engine  *gossip.Engine
	Members *membership.List
}

type Cluster struct {
	nodes []*NodeHandle

	basePort int
	l        *logger.Logger
}

type ClusterConfig struct {
	NumNodes          int
	NumZones          int
	BasePort          int
	PacketLossPercent float64
	GossipInterval    time.Duration
}

func Default() ClusterConfig {
	return ClusterConfig{
		NumNodes:       10,
		NumZones:       3,
		BasePort:       10000,
		GossipInterval: 200 * time.Millisecond,
	}
}

func NewCluster(cc ClusterConfig) (*Cluster, error) {
	l := logger.New(logger.LevelProd)

	c := &Cluster{
		basePort: cc.BasePort,
		l:        l,
	}

	seedAddrs := make([]string, 0)

	for i := 0; i < cc.NumNodes; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		zone := fmt.Sprintf("zone-%d", i%cc.NumZones)
		port := cc.BasePort + i
		bindAddr := fmt.Sprintf("127.0.0.1:%d", port)
		apiAddr := fmt.Sprintf("127.0.0.1:%d", port+20000)

		cfg := config.Default()
		cfg.NodeID = nodeID
		cfg.BindAddr = bindAddr
		cfg.APIAddr = apiAddr
		cfg.Zone = zone
		cfg.GossipInterval = cc.GossipInterval
		cfg.JoinAddrs = seedAddrs
		cfg.HealthcheckURL = ""

		members := membership.NewList(nodeID, bindAddr, zone)
		engine, err := gossip.NewEngine(cfg, members, l.With(logger.Param("node", nodeID)))
		if err != nil {
			c.Shutdown()
			return nil, fmt.Errorf("create engine for %s: %w", nodeID, err)
		}

		c.nodes = append(c.nodes, &NodeHandle{
			ID:      nodeID,
			Engine:  engine,
			Members: members,
		})

		if i < 3 {
			seedAddrs = append(seedAddrs, bindAddr)
		}
	}

	return c, nil
}

func (c *Cluster) Start() {
	for _, n := range c.nodes {
		n.Engine.Start()
	}
	time.Sleep(100 * time.Millisecond)

	for _, n := range c.nodes {
		seeds := c.getSeedAddrs(n.ID)
		if len(seeds) > 0 {
			_ = n.Engine.Join(seeds)
		}
	}
}

func (c *Cluster) getSeedAddrs(selfID string) []string {
	var addrs []string
	for i, n := range c.nodes {
		if n.ID == selfID {
			continue
		}
		addrs = append(addrs, fmt.Sprintf("127.0.0.1:%d", c.basePort+i))
		if len(addrs) >= 3 {
			break
		}
	}
	return addrs
}

func (c *Cluster) Shutdown() {
	for _, n := range c.nodes {
		n.Engine.Stop()
	}
}

func (c *Cluster) Nodes() []*NodeHandle {
	return c.nodes
}

func (c *Cluster) KillNode(idx int) {
	if idx < 0 || idx >= len(c.nodes) {
		return
	}
	c.nodes[idx].Engine.Stop()
	c.l.Info("killed node", logger.Param("node", c.nodes[idx].ID))
}

// WaitForFullMembership waits until every node sees all other nodes as alive.
func (c *Cluster) WaitForFullMembership(timeout time.Duration) (time.Duration, bool) {
	start := time.Now()
	deadline := time.After(timeout)

	for {
		select {
		case <-deadline:
			return time.Since(start), false
		default:
			if c.hasFullMembership() {
				return time.Since(start), true
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (c *Cluster) hasFullMembership() bool {
	expected := len(c.nodes)
	for _, n := range c.nodes {
		if len(n.Members.AliveMembers()) < expected {
			return false
		}
	}
	return true
}

// WaitForConvergence waits until all alive nodes agree on the state of the target node.
func (c *Cluster) WaitForConvergence(
	targetID string, expectedState pb.NodeState, timeout time.Duration,
) (time.Duration, bool) {
	start := time.Now()
	deadline := time.After(timeout)

	for {
		select {
		case <-deadline:
			return time.Since(start), false
		default:
			if c.isConverged(targetID, expectedState) {
				return time.Since(start), true
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (c *Cluster) isConverged(targetID string, expectedState pb.NodeState) bool {
	for _, n := range c.nodes {
		if n.ID == targetID {
			continue
		}
		node, ok := n.Members.Get(targetID)
		if !ok {
			if expectedState == pb.NodeState_NODE_STATE_DEAD {
				continue
			}
			return false
		}
		if node.State != expectedState {
			return false
		}
	}
	return true
}

// MeasureConvergence returns the time for all nodes to learn about a state change.
func MeasureConvergence(numNodes int, numZones int) (time.Duration, error) {
	cc := ClusterConfig{
		NumNodes:       numNodes,
		NumZones:       numZones,
		BasePort:       30000 + cryptoIntn(10000),
		GossipInterval: 100 * time.Millisecond,
	}

	cluster, err := NewCluster(cc)
	if err != nil {
		return 0, err
	}
	defer cluster.Shutdown()

	cluster.Start()

	propagationTime := time.Duration(numNodes) * 10 * time.Millisecond
	if propagationTime < 3*time.Second {
		propagationTime = 3 * time.Second
	}
	time.Sleep(propagationTime)

	targetNode := cluster.Nodes()[numNodes-1]
	cluster.KillNode(numNodes - 1)

	convergenceTime, converged := cluster.WaitForConvergence(
		targetNode.ID, pb.NodeState_NODE_STATE_DEAD, 30*time.Second,
	)

	if !converged {
		return convergenceTime, fmt.Errorf("convergence not reached within timeout")
	}
	return convergenceTime, nil
}

func cryptoIntn(max int) int {
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
