package gossip

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"google.golang.org/protobuf/proto"
)

const maxUDPPacketSize = 1400

type Transport struct {
	conn *net.UDPConn
	l    *logger.Logger

	msgCh  chan *incomingMsg
	closed atomic.Bool
	wg     sync.WaitGroup
}

type incomingMsg struct {
	msg  *pb.GossipMessage
	from *net.UDPAddr
}

func NewTransport(bindAddr string, l *logger.Logger) (*Transport, error) {
	addr, err := net.ResolveUDPAddr("udp", bindAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}

	t := &Transport{
		conn:  conn,
		l:     l,
		msgCh: make(chan *incomingMsg, 256),
	}

	t.wg.Add(1)
	go t.readLoop()

	return t, nil
}

func (t *Transport) readLoop() {
	defer t.wg.Done()

	buf := make([]byte, maxUDPPacketSize)
	for {
		n, from, err := t.conn.ReadFromUDP(buf)
		if err != nil {
			if t.closed.Load() {
				return
			}
			t.l.Debug("UDP read error", logger.Error(err))
			continue
		}

		msg := &pb.GossipMessage{}
		if err := proto.Unmarshal(buf[:n], msg); err != nil {
			t.l.Debug("unmarshal error", logger.Error(err), logger.Param("from", from))
			continue
		}

		select {
		case t.msgCh <- &incomingMsg{msg: msg, from: from}:
		default:
			t.l.Warn("message channel full, dropping message")
		}
	}
}

func (t *Transport) Messages() <-chan *incomingMsg {
	return t.msgCh
}

func (t *Transport) SendTo(msg *pb.GossipMessage, addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve addr %s: %w", addr, err)
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if len(data) > maxUDPPacketSize {
		t.l.Warn("message exceeds MTU, sending anyway", logger.Param("size", len(data)))
	}

	_, err = t.conn.WriteToUDP(data, udpAddr)
	return err
}

func (t *Transport) LocalAddr() string {
	return t.conn.LocalAddr().String()
}

func (t *Transport) Close() error {
	t.closed.Store(true)
	err := t.conn.Close()
	t.wg.Wait()
	close(t.msgCh)
	return err
}
