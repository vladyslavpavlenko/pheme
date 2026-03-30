package gossip

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vladyslavpavlenko/pheme/internal/bloom"
	"github.com/vladyslavpavlenko/pheme/internal/config"
	"github.com/vladyslavpavlenko/pheme/internal/detector"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
	"github.com/vladyslavpavlenko/pheme/internal/topology"
)

type Engine struct {
	cfg       *config.Config
	members   *membership.List
	transport *Transport
	selector  *topology.Selector
	detector  *detector.Detector
	bloom     *bloom.Filter
	l         *logger.Logger

	seqNum atomic.Uint64

	pendingMu   sync.Mutex
	pendingAcks map[uint64]*pendingPing

	stopped atomic.Bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

type pendingPing struct {
	target   string
	sentAt   time.Time
	timer    *time.Timer
	indirect bool
	relayCh  chan struct{}
}

func NewEngine(cfg *config.Config, members *membership.List, l *logger.Logger) (*Engine, error) {
	transport, err := NewTransport(cfg.BindAddr, l)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	e := &Engine{
		cfg:       cfg,
		members:   members,
		transport: transport,
		selector:  topology.NewSelector(cfg.NodeID, cfg.Zone, cfg.LocalWeight),
		detector:  detector.New(cfg.RTTWindowSize, cfg.SuspicionMultiplier),
		bloom:     bloom.New(cfg.BloomExpectedItems, cfg.BloomFPRate, cfg.BloomRotationInterval),
		l:         l,

		pendingAcks: make(map[uint64]*pendingPing),
		stopCh:      make(chan struct{}),
	}

	return e, nil
}

func (e *Engine) Start() {
	e.bloom.Start()
	e.wg.Add(2)
	go e.gossipLoop()
	go e.messageHandler()
}

func (e *Engine) Stop() {
	if e.stopped.Swap(true) {
		return
	}
	close(e.stopCh)
	e.bloom.Stop()
	_ = e.transport.Close()
	e.wg.Wait()
}

func (e *Engine) Members() *membership.List {
	return e.members
}

func (e *Engine) Join(addrs []string) error {
	self := e.members.GetSelf()
	for _, addr := range addrs {
		seq := e.seqNum.Add(1)
		msg := &pb.GossipMessage{
			Payload: &pb.GossipMessage_Ping{
				Ping: &pb.Ping{
					SenderId:    self.ID,
					SequenceNum: seq,
					Updates: []*pb.StateUpdate{
						{
							NodeId:  self.ID,
							State:   pb.NodeState_NODE_STATE_ALIVE,
							Version: self.Version,
							Zone:    self.Zone,
							Addr:    e.transport.LocalAddr(),
						},
					},
				},
			},
		}
		if err := e.transport.SendTo(msg, addr); err != nil {
			e.l.Warn("failed to send join ping", logger.Param("addr", addr), logger.Error(err))
		}
	}
	return nil
}

func (e *Engine) gossipLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.cfg.GossipInterval)
	defer ticker.Stop()

	rejoinTicker := time.NewTicker(5 * time.Second)
	defer rejoinTicker.Stop()

	for {
		select {
		case <-ticker.C:
			e.doGossipRound()
		case <-rejoinTicker.C:
			e.maybeRejoin()
		case <-e.stopCh:
			return
		}
	}
}

// maybeRejoin re-attempts joining seed nodes when this node is isolated.
func (e *Engine) maybeRejoin() {
	if len(e.cfg.JoinAddrs) == 0 {
		return
	}
	members := e.members.AliveMembers()
	if len(members) > 1 {
		return
	}
	e.l.Info("node is isolated, re-attempting join", logger.Param("seeds", e.cfg.JoinAddrs))
	_ = e.Join(e.cfg.JoinAddrs)
}

func (e *Engine) doGossipRound() {
	members := e.members.AliveMembers()
	target := e.selector.Select(members)
	if target == nil {
		return
	}

	updates := e.getFilteredUpdates()

	seq := e.seqNum.Add(1)
	ping := &pb.GossipMessage{
		Payload: &pb.GossipMessage_Ping{
			Ping: &pb.Ping{
				SenderId:    e.cfg.NodeID,
				SequenceNum: seq,
				Updates:     updates,
			},
		},
	}

	timeout := e.detector.PingTimeout(target.ID)

	e.pendingMu.Lock()
	timer := time.AfterFunc(timeout, func() {
		e.handlePingTimeout(seq, target)
	})
	e.pendingAcks[seq] = &pendingPing{
		target: target.ID,
		sentAt: time.Now(),
		timer:  timer,
	}
	e.pendingMu.Unlock()

	if err := e.transport.SendTo(ping, target.Addr); err != nil {
		e.l.Debug("failed to send ping", logger.Param("target", target.ID), logger.Error(err))
	}
}

func (e *Engine) handlePingTimeout(seq uint64, target *membership.Node) {
	e.pendingMu.Lock()
	_, ok := e.pendingAcks[seq]
	if ok {
		delete(e.pendingAcks, seq)
	}
	e.pendingMu.Unlock()

	if !ok {
		return
	}

	e.l.Debug("ping timeout, starting indirect probe", logger.Param("target", target.ID))
	e.doIndirectProbe(target)
}

func (e *Engine) doIndirectProbe(target *membership.Node) {
	members := e.members.AliveMembers()
	mediators := e.selector.SelectN(members, e.cfg.IndirectChecks)

	indirectSeqs := make([]uint64, 0, len(mediators))

	for _, med := range mediators {
		if med.ID == target.ID {
			continue
		}
		seq := e.seqNum.Add(1)
		indirectSeqs = append(indirectSeqs, seq)

		reqMsg := &pb.GossipMessage{
			Payload: &pb.GossipMessage_PingReq{
				PingReq: &pb.PingReq{
					SenderId:    e.cfg.NodeID,
					TargetId:    target.ID,
					SequenceNum: seq,
				},
			},
		}

		e.pendingMu.Lock()
		e.pendingAcks[seq] = &pendingPing{
			target:   target.ID,
			sentAt:   time.Now(),
			indirect: true,
		}
		e.pendingMu.Unlock()

		if err := e.transport.SendTo(reqMsg, med.Addr); err != nil {
			e.l.Debug("failed to send ping-req", logger.Param("mediator", med.ID), logger.Error(err))
		}
	}

	if len(indirectSeqs) == 0 {
		e.l.Info("node suspected (no mediators available)", logger.Param("node", target.ID))
		e.members.SetState(target.ID, pb.NodeState_NODE_STATE_SUSPECT)
		e.scheduleDeath(target.ID)
		return
	}

	suspicionTimeout := e.detector.SuspicionTimeout(target.ID)

	time.AfterFunc(suspicionTimeout, func() {
		e.pendingMu.Lock()
		anyAcked := false
		for _, seq := range indirectSeqs {
			if _, stillPending := e.pendingAcks[seq]; !stillPending {
				anyAcked = true
			}
			delete(e.pendingAcks, seq)
		}
		e.pendingMu.Unlock()

		if !anyAcked {
			e.l.Info("node suspected", logger.Param("node", target.ID))
			e.members.SetState(target.ID, pb.NodeState_NODE_STATE_SUSPECT)
			e.scheduleDeath(target.ID)
		}
	})
}

func (e *Engine) scheduleDeath(nodeID string) {
	timeout := e.detector.SuspicionTimeout(nodeID)

	time.AfterFunc(timeout, func() {
		n, ok := e.members.Get(nodeID)
		if !ok {
			return
		}
		if n.State == pb.NodeState_NODE_STATE_SUSPECT {
			e.l.Info("node declared dead", logger.Param("node", nodeID))
			e.members.SetState(nodeID, pb.NodeState_NODE_STATE_DEAD)
		}
	})
}

func (e *Engine) messageHandler() {
	defer e.wg.Done()

	for {
		select {
		case incoming, ok := <-e.transport.Messages():
			if !ok {
				return
			}
			e.handleMessage(incoming)
		case <-e.stopCh:
			return
		}
	}
}

func (e *Engine) handleMessage(incoming *incomingMsg) {
	msg := incoming.msg

	switch p := msg.Payload.(type) {
	case *pb.GossipMessage_Ping:
		e.handlePing(p.Ping, incoming.from.String())
	case *pb.GossipMessage_Ack:
		e.handleAck(p.Ack)
	case *pb.GossipMessage_PingReq:
		e.handlePingReq(p.PingReq, incoming.from.String())
	}
}

func (e *Engine) handlePing(ping *pb.Ping, fromAddr string) {
	for _, u := range ping.Updates {
		fp := bloom.Fingerprint(u.NodeId, int32(u.State), u.Version)
		if e.bloom.AddIfNotPresent(fp) {
			e.members.ApplyUpdate(u)
		}
	}

	updates := e.getFilteredUpdates()
	ack := &pb.GossipMessage{
		Payload: &pb.GossipMessage_Ack{
			Ack: &pb.Ack{
				ResponderId: e.cfg.NodeID,
				SequenceNum: ping.SequenceNum,
				Updates:     updates,
			},
		},
	}

	if err := e.transport.SendTo(ack, fromAddr); err != nil {
		e.l.Debug("failed to send ack", logger.Param("to", fromAddr), logger.Error(err))
	}
}

func (e *Engine) handleAck(ack *pb.Ack) {
	for _, u := range ack.Updates {
		fp := bloom.Fingerprint(u.NodeId, int32(u.State), u.Version)
		if e.bloom.AddIfNotPresent(fp) {
			e.members.ApplyUpdate(u)
		}
	}

	e.pendingMu.Lock()
	pending, ok := e.pendingAcks[ack.SequenceNum]
	if ok {
		delete(e.pendingAcks, ack.SequenceNum)
		if pending.timer != nil {
			pending.timer.Stop()
		}
	}
	e.pendingMu.Unlock()

	if !ok {
		return
	}

	if pending.relayCh != nil {
		select {
		case pending.relayCh <- struct{}{}:
		default:
		}
	}

	if !pending.indirect {
		rtt := time.Since(pending.sentAt)
		e.detector.RecordRTT(pending.target, rtt)

		// Only refute gossip-level suspicion when the target still
		// considers itself ALIVE. If the target self-declared SUSPECT
		// (e.g. local health check failure), respect that — the ack's
		// piggybacked updates already carry the authoritative state.
		targetSelfState := pb.NodeState_NODE_STATE_UNSPECIFIED
		for _, u := range ack.Updates {
			if u.NodeId == pending.target {
				targetSelfState = u.State
				break
			}
		}
		if targetSelfState == pb.NodeState_NODE_STATE_ALIVE {
			n, exists := e.members.Get(pending.target)
			if exists && n.State == pb.NodeState_NODE_STATE_SUSPECT {
				e.members.SetState(pending.target, pb.NodeState_NODE_STATE_ALIVE)
			}
		}
	}
}

func (e *Engine) handlePingReq(req *pb.PingReq, fromAddr string) {
	target, ok := e.members.Get(req.TargetId)
	if !ok {
		return
	}

	seq := e.seqNum.Add(1)
	ping := &pb.GossipMessage{
		Payload: &pb.GossipMessage_Ping{
			Ping: &pb.Ping{
				SenderId:    e.cfg.NodeID,
				SequenceNum: seq,
			},
		},
	}

	forwardTimeout := e.detector.PingTimeout(req.TargetId)

	gotAck := make(chan struct{}, 1)

	e.pendingMu.Lock()
	timer := time.AfterFunc(forwardTimeout, func() {
		e.pendingMu.Lock()
		delete(e.pendingAcks, seq)
		e.pendingMu.Unlock()
	})
	e.pendingAcks[seq] = &pendingPing{
		target:   req.TargetId,
		sentAt:   time.Now(),
		timer:    timer,
		indirect: true,
		relayCh:  gotAck,
	}
	e.pendingMu.Unlock()

	if err := e.transport.SendTo(ping, target.Addr); err != nil {
		e.l.Debug("failed to forward ping-req", logger.Param("target", target.ID), logger.Error(err))
		return
	}

	go func() {
		select {
		case <-gotAck:
			ack := &pb.GossipMessage{
				Payload: &pb.GossipMessage_Ack{
					Ack: &pb.Ack{
						ResponderId: target.ID,
						SequenceNum: req.SequenceNum,
					},
				},
			}
			if err := e.transport.SendTo(ack, fromAddr); err != nil {
				e.l.Debug("failed to relay ack", logger.Param("to", fromAddr), logger.Error(err))
			}
		case <-time.After(forwardTimeout + 50*time.Millisecond):
			// target didn't respond; don't relay anything
		}
	}()
}

func (e *Engine) getFilteredUpdates() []*pb.StateUpdate {
	all := e.members.GetUpdates()
	var filtered []*pb.StateUpdate
	totalSize := 0
	maxPayload := maxUDPPacketSize - 200

	for _, u := range all {
		estimatedSize := 20 + len(u.NodeId) + len(u.Zone) + len(u.Addr)
		if totalSize+estimatedSize > maxPayload {
			break
		}
		filtered = append(filtered, u)
		totalSize += estimatedSize
	}

	return filtered
}
