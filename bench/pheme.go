package bench

import (
	crand "crypto/rand"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/vladyslavpavlenko/pheme/internal/config"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/gossip"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

type phemeNode struct {
	id      string
	engine  *gossip.Engine
	members *membership.List
}

func RunPheme(numNodes int) (*Result, error) {
	basePort := 10000 + cryptoIntn(5000)
	interval := 200 * time.Millisecond

	l := logger.NewNop()

	nodes := make([]*phemeNode, 0, numNodes)
	var seedAddrs []string

	defer func() {
		for _, n := range nodes {
			n.engine.Stop()
		}
	}()

	fmt.Fprintf(os.Stderr, "[pheme] creating %d-node cluster (ports %d–%d)...\n",
		numNodes, basePort, basePort+numNodes-1)

	for i := 0; i < numNodes; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		zone := fmt.Sprintf("zone-%d", i%5)
		bindAddr := fmt.Sprintf("127.0.0.1:%d", basePort+i)

		cfg := config.Default()
		cfg.NodeID = nodeID
		cfg.BindAddr = bindAddr
		cfg.APIAddr = fmt.Sprintf("127.0.0.1:%d", basePort+numNodes+i)
		cfg.Zone = zone
		cfg.GossipInterval = interval
		cfg.IndirectChecks = 3
		cfg.SuspicionMultiplier = 4
		cfg.JoinAddrs = seedAddrs
		cfg.HealthcheckURL = ""

		members := membership.NewList(nodeID, bindAddr, zone)
		engine, err := gossip.NewEngine(cfg, members, l)
		if err != nil {
			return nil, fmt.Errorf("create engine for %s: %w", nodeID, err)
		}

		nodes = append(nodes, &phemeNode{id: nodeID, engine: engine, members: members})

		if i < 3 {
			seedAddrs = append(seedAddrs, bindAddr)
		}
	}

	for _, n := range nodes {
		n.engine.Start()
	}
	time.Sleep(50 * time.Millisecond)

	for i, n := range nodes {
		if i == 0 {
			continue
		}
		seeds := make([]string, 0, 3)
		for j := 0; j < i && j < 3; j++ {
			seeds = append(seeds, fmt.Sprintf("127.0.0.1:%d", basePort+j))
		}
		_ = n.engine.Join(seeds)
	}

	res := &Result{System: "Pheme", Nodes: numNodes}

	// convergence
	convTimeout := time.Duration(numNodes)*4*time.Second + 60*time.Second
	start := time.Now()
	if err := phemeWaitFull(nodes, convTimeout); err != nil {
		return nil, fmt.Errorf("convergence: %w", err)
	}
	res.Convergence = time.Since(start)
	fmt.Fprintf(os.Stderr, "[pheme] convergence: %v\n", res.Convergence)

	// failure detection: kill last node
	targetIdx := numNodes - 1
	target := nodes[targetIdx]
	fmt.Fprintf(os.Stderr, "[pheme] killing %s...\n", target.id)
	target.engine.Stop()

	detectTimeout := time.Duration(numNodes)*time.Second + 30*time.Second
	start = time.Now()
	if err := phemeWaitState(nodes, target.id, targetIdx,
		[]pb.NodeState{pb.NodeState_NODE_STATE_SUSPECT, pb.NodeState_NODE_STATE_DEAD},
		detectTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[pheme] warning: failure detection timed out: %v\n", err)
		res.FailureDetect = detectTimeout
	} else {
		res.FailureDetect = time.Since(start)
	}
	fmt.Fprintf(os.Stderr, "[pheme] failure detection: %v\n", res.FailureDetect)

	// wait for broadcast queues to drain before measuring steady-state bandwidth
	fmt.Fprintf(os.Stderr, "[pheme] waiting for broadcast queues to drain...\n")
	drainDeadline := time.Now().Add(time.Duration(numNodes)*time.Second + 30*time.Second)
	for time.Now().Before(drainDeadline) {
		maxQueue := 0
		for i := 0; i < targetIdx; i++ {
			if ql := nodes[i].members.BroadcastQueueLen(); ql > maxQueue {
				maxQueue = ql
			}
		}
		if maxQueue == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// bandwidth (5s window, skip killed node)
	fmt.Fprintf(os.Stderr, "[pheme] measuring bandwidth (5s)...\n")
	before := make([]gossip.TransportStats, targetIdx)
	for i := 0; i < targetIdx; i++ {
		before[i] = nodes[i].engine.Stats()
	}
	time.Sleep(5 * time.Second)
	var totalDelta uint64
	for i := 0; i < targetIdx; i++ {
		after := nodes[i].engine.Stats()
		totalDelta += (after.BytesSent - before[i].BytesSent) +
			(after.BytesRecv - before[i].BytesRecv)
	}
	res.BandwidthPerNode = float64(totalDelta) / float64(targetIdx) / 5.0
	fmt.Fprintf(os.Stderr, "[pheme] bandwidth/node: %.0f B/s\n", res.BandwidthPerNode)

	return res, nil
}

func phemeWaitFull(nodes []*phemeNode, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	expected := len(nodes)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("not all nodes converged within %v", timeout)
		}
		allGood := true
		for _, n := range nodes {
			if len(n.members.AliveMembers()) < expected {
				allGood = false
				break
			}
		}
		if allGood {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func phemeWaitState(nodes []*phemeNode, targetID string, skipIdx int, wantStates []pb.NodeState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("node %s did not reach states %v within %v", targetID, wantStates, timeout)
		}
		allSee := true
		for i, n := range nodes {
			if i == skipIdx {
				continue
			}
			node, ok := n.members.Get(targetID)
			if !ok {
				allSee = false
				break
			}
			found := false
			for _, ws := range wantStates {
				if node.State == ws {
					found = true
					break
				}
			}
			if !found {
				allSee = false
				break
			}
		}
		if allSee {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func cryptoIntn(m int) int {
	n, err := crand.Int(crand.Reader, big.NewInt(int64(m)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
