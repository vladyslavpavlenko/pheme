package grpc

import (
	"context"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedClusterServiceServer
	members *membership.List
	l       *logger.Logger
}

func NewServer(members *membership.List, l *logger.Logger) *Server {
	return &Server{members: members, l: l}
}

func (s *Server) GetClusterStatus(
	_ context.Context, _ *pb.GetClusterStatusRequest,
) (*pb.GetClusterStatusResponse, error) {
	nodes := s.members.AllNodes()
	resp := &pb.GetClusterStatusResponse{}
	for _, n := range nodes {
		resp.Nodes = append(resp.Nodes, &pb.NodeInfo{
			NodeId:  n.ID,
			Addr:    n.Addr,
			State:   n.State,
			Zone:    n.Zone,
			Version: n.Version,
		})
	}
	return resp, nil
}

func (s *Server) GetMembers(_ context.Context, _ *pb.GetMembersRequest) (*pb.GetMembersResponse, error) {
	nodes := s.members.AliveMembers()
	resp := &pb.GetMembersResponse{}
	for _, n := range nodes {
		resp.Members = append(resp.Members, &pb.NodeInfo{
			NodeId:  n.ID,
			Addr:    n.Addr,
			State:   n.State,
			Zone:    n.Zone,
			Version: n.Version,
		})
	}
	return resp, nil
}

func (s *Server) WatchState(_ *pb.WatchStateRequest, stream grpc.ServerStreamingServer[pb.WatchStateResponse]) error {
	ch := s.members.Subscribe()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case change, ok := <-ch:
			if !ok {
				return nil
			}
			node, exists := s.members.Get(change.NodeID)
			if !exists {
				continue
			}
			if err := stream.Send(&pb.WatchStateResponse{
				NodeId:  node.ID,
				State:   node.State,
				Version: node.Version,
				Zone:    node.Zone,
				Addr:    node.Addr,
			}); err != nil {
				return err
			}
		}
	}
}
