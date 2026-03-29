package grpc_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	grpcapi "github.com/vladyslavpavlenko/pheme/internal/api/grpc"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

func TestGetClusterStatus(t *testing.T) {
	srv := newTestServer()

	resp, err := srv.GetClusterStatus(context.Background(), &pb.GetClusterStatusRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Nodes, 3, "should return all nodes including dead")

	ids := make(map[string]bool)
	for _, n := range resp.Nodes {
		ids[n.NodeId] = true
	}
	assert.True(t, ids["node-1"])
	assert.True(t, ids["node-2"])
	assert.True(t, ids["node-3"])
}

func TestGetMembers(t *testing.T) {
	srv := newTestServer()

	resp, err := srv.GetMembers(context.Background(), &pb.GetMembersRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Members, 2, "should return only alive/suspect nodes")

	for _, m := range resp.Members {
		assert.NotEqual(t, pb.NodeState_NODE_STATE_DEAD, m.State)
	}
}

func TestWatchState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		members := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
		members.ApplyUpdate(&pb.StateUpdate{
			NodeId: "node-2", Addr: "127.0.0.1:7947",
			State: pb.NodeState_NODE_STATE_ALIVE, Version: 1, Zone: "zone-b",
		})
		srv := grpcapi.NewServer(members, logger.NewNop())

		ctx, cancel := context.WithCancel(t.Context())
		stream := new(streamMock)
		stream.On("Context").Return(ctx)

		var got *pb.WatchStateResponse
		stream.On("Send", mock.Anything).
			Run(func(args mock.Arguments) {
				got = args.Get(0).(*pb.WatchStateResponse)
			}).
			Return(nil)

		done := make(chan error, 1)
		go func() {
			done <- srv.WatchState(&pb.WatchStateRequest{}, stream)
		}()

		synctest.Wait()

		members.SetState("node-2", pb.NodeState_NODE_STATE_SUSPECT)
		synctest.Wait()

		require.NotNil(t, got)
		assert.Equal(t, "node-2", got.NodeId)
		assert.Equal(t, pb.NodeState_NODE_STATE_SUSPECT, got.State)

		cancel()
		synctest.Wait()

		err := <-done
		assert.ErrorIs(t, err, context.Canceled)
		stream.AssertExpectations(t)
	})
}

func newTestServer() *grpcapi.Server {
	members := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")

	members.ApplyUpdate(&pb.StateUpdate{
		NodeId: "node-2", Addr: "127.0.0.1:7947",
		State: pb.NodeState_NODE_STATE_ALIVE, Version: 1, Zone: "zone-b",
	})
	members.ApplyUpdate(&pb.StateUpdate{
		NodeId: "node-3", Addr: "127.0.0.1:7948",
		State: pb.NodeState_NODE_STATE_DEAD, Version: 1, Zone: "zone-a",
	})

	return grpcapi.NewServer(members, logger.NewNop())
}

type streamMock struct {
	mock.Mock
}

func (m *streamMock) Send(resp *pb.WatchStateResponse) error {
	return m.Called(resp).Error(0)
}

func (m *streamMock) Context() context.Context {
	return m.Called().Get(0).(context.Context)
}

func (m *streamMock) SetHeader(metadata.MD) error  { return nil }
func (m *streamMock) SendHeader(metadata.MD) error { return nil }
func (m *streamMock) SetTrailer(metadata.MD)       {}
func (m *streamMock) RecvMsg(any) error            { return nil }
func (m *streamMock) SendMsg(any) error            { return nil }
