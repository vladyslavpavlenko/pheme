package bench

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/hashicorp/memberlist"
)

type mlNode struct {
	list    *memberlist.Memberlist
	name    string
	counter *byteCounter
}

func RunMemberlist(numNodes int) (*Result, error) {
	basePort := 40000 + cryptoIntn(5000)

	nodes := make([]*mlNode, 0, numNodes)
	defer func() {
		for _, n := range nodes {
			_ = n.list.Shutdown()
		}
	}()

	fmt.Fprintf(os.Stderr, "[memberlist] creating %d-node cluster (ports %d–%d)...\n",
		numNodes, basePort, basePort+numNodes-1)

	for i := 0; i < numNodes; i++ {
		cfg := memberlist.DefaultLocalConfig()
		cfg.Name = fmt.Sprintf("ml-node-%d", i)
		cfg.BindAddr = "127.0.0.1"
		cfg.BindPort = basePort + i
		cfg.AdvertiseAddr = "127.0.0.1"
		cfg.AdvertisePort = basePort + i
		cfg.LogOutput = io.Discard
		cfg.Logger = nil

		// Match Pheme's parameters for fair comparison.
		// memberlist's ProbeInterval is the main gossip cycle (like Pheme's GossipInterval).
		// GossipInterval in memberlist is only for queued broadcast messages.
		cfg.ProbeInterval = 200 * time.Millisecond
		cfg.ProbeTimeout = 100 * time.Millisecond
		cfg.GossipInterval = 100 * time.Millisecond
		cfg.IndirectChecks = 3
		cfg.SuspicionMult = 4

		counter := &byteCounter{}

		nc := &memberlist.NetTransportConfig{
			BindAddrs: []string{"127.0.0.1"},
			BindPort:  basePort + i,
			Logger:    log.New(io.Discard, "", 0),
		}
		nt, err := memberlist.NewNetTransport(nc)
		if err != nil {
			return nil, fmt.Errorf("create transport for node %d: %w", i, err)
		}
		cfg.Transport = newCountingTransport(nt, counter)

		list, err := memberlist.Create(cfg)
		if err != nil {
			return nil, fmt.Errorf("create memberlist node %d: %w", i, err)
		}

		nodes = append(nodes, &mlNode{list: list, name: cfg.Name, counter: counter})
	}

	for i := 1; i < numNodes; i++ {
		seeds := make([]string, 0, 3)
		for j := 0; j < i && j < 3; j++ {
			seeds = append(seeds, fmt.Sprintf("127.0.0.1:%d", basePort+j))
		}
		if _, err := nodes[i].list.Join(seeds); err != nil {
			fmt.Fprintf(os.Stderr, "[memberlist] warning: node %d join failed: %v\n", i, err)
		}
	}

	res := &Result{System: "memberlist", Nodes: numNodes}

	// convergence
	convTimeout := time.Duration(numNodes)*2*time.Second + 30*time.Second
	start := time.Now()
	if err := mlWaitFull(nodes, numNodes, convTimeout); err != nil {
		return nil, fmt.Errorf("convergence: %w", err)
	}
	res.Convergence = time.Since(start)
	fmt.Fprintf(os.Stderr, "[memberlist] convergence: %v\n", res.Convergence)

	// failure detection: shut down last node
	targetIdx := numNodes - 1
	target := nodes[targetIdx]
	fmt.Fprintf(os.Stderr, "[memberlist] killing %s...\n", target.name)
	_ = target.list.Shutdown()

	detectTimeout := time.Duration(numNodes)*time.Second + 30*time.Second
	start = time.Now()
	if err := mlWaitGone(nodes, target.name, targetIdx, detectTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "[memberlist] warning: failure detection timed out: %v\n", err)
		res.FailureDetect = detectTimeout
	} else {
		res.FailureDetect = time.Since(start)
	}
	fmt.Fprintf(os.Stderr, "[memberlist] failure detection: %v\n", res.FailureDetect)

	// bandwidth (5s window, skip killed node)
	fmt.Fprintf(os.Stderr, "[memberlist] measuring bandwidth (5s)...\n")
	for i := 0; i < targetIdx; i++ {
		nodes[i].counter.reset()
	}
	time.Sleep(5 * time.Second)
	var totalBytes uint64
	for i := 0; i < targetIdx; i++ {
		totalBytes += nodes[i].counter.sent.Load() + nodes[i].counter.recv.Load()
	}
	res.BandwidthPerNode = float64(totalBytes) / float64(targetIdx) / 5.0
	fmt.Fprintf(os.Stderr, "[memberlist] bandwidth/node: %.0f B/s\n", res.BandwidthPerNode)

	return res, nil
}

func mlWaitFull(nodes []*mlNode, expected int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("not all nodes converged within %v", timeout)
		}
		allGood := true
		for _, n := range nodes {
			if n.list.NumMembers() < expected {
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

func mlWaitGone(nodes []*mlNode, targetName string, skipIdx int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("node %s not detected as failed within %v", targetName, timeout)
		}
		allSee := true
		for i, n := range nodes {
			if i == skipIdx {
				continue
			}
			for _, m := range n.list.Members() {
				if m.Name == targetName && m.State == memberlist.StateAlive {
					allSee = false
					break
				}
			}
			if !allSee {
				break
			}
		}
		if allSee {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// byteCounter tracks cumulative sent/received bytes.
type byteCounter struct {
	sent atomic.Uint64
	recv atomic.Uint64
}

func (b *byteCounter) reset() {
	b.sent.Store(0)
	b.recv.Store(0)
}

// countingTransport wraps memberlist.NetTransport and counts bytes on UDP.
type countingTransport struct {
	inner    *memberlist.NetTransport
	counter  *byteCounter
	packetCh chan *memberlist.Packet
}

func newCountingTransport(inner *memberlist.NetTransport, counter *byteCounter) *countingTransport {
	ct := &countingTransport{
		inner:    inner,
		counter:  counter,
		packetCh: make(chan *memberlist.Packet, 1024),
	}
	go ct.forwardPackets()
	return ct
}

func (t *countingTransport) forwardPackets() {
	for pkt := range t.inner.PacketCh() {
		t.counter.recv.Add(uint64(len(pkt.Buf)))
		t.packetCh <- pkt
	}
	close(t.packetCh)
}

func (t *countingTransport) FinalAdvertiseAddr(ip string, port int) (net.IP, int, error) {
	return t.inner.FinalAdvertiseAddr(ip, port)
}

func (t *countingTransport) WriteTo(b []byte, addr string) (time.Time, error) {
	ts, err := t.inner.WriteTo(b, addr)
	if err == nil {
		t.counter.sent.Add(uint64(len(b)))
	}
	return ts, err
}

func (t *countingTransport) PacketCh() <-chan *memberlist.Packet {
	return t.packetCh
}

func (t *countingTransport) DialTimeout(addr string, timeout time.Duration) (net.Conn, error) {
	return t.inner.DialTimeout(addr, timeout)
}

func (t *countingTransport) StreamCh() <-chan net.Conn {
	return t.inner.StreamCh()
}

func (t *countingTransport) Shutdown() error {
	return t.inner.Shutdown()
}
