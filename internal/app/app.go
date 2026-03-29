package app

import (
	"context"
	"fmt"
	"net"
	nethttp "net/http"
	"os"
	"time"

	grpcapi "github.com/vladyslavpavlenko/pheme/internal/api/grpc"
	restapi "github.com/vladyslavpavlenko/pheme/internal/api/rest"
	"github.com/vladyslavpavlenko/pheme/internal/config"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/gossip"
	"github.com/vladyslavpavlenko/pheme/internal/healthcheck"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type App struct {
	cfg *config.Config
	l   *logger.Logger

	members    *membership.List
	engine     *gossip.Engine
	hc         *healthcheck.Checker
	httpServer *nethttp.Server
	grpcServer *grpc.Server
}

func New(cfg *config.Config, l *logger.Logger) (*App, error) {
	if cfg.NodeID == "" {
		hostname, _ := os.Hostname()
		cfg.NodeID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	l = l.With(logger.Param("node", cfg.NodeID))

	members := membership.NewList(cfg.NodeID, cfg.BindAddr, cfg.Zone)

	engine, err := gossip.NewEngine(cfg, members, l)
	if err != nil {
		return nil, fmt.Errorf("create gossip engine: %w", err)
	}

	hc := healthcheck.New(cfg.HealthcheckURL, cfg.HealthcheckInterval, l)

	httpHandler := restapi.NewHandler(members, l)
	mux := nethttp.NewServeMux()
	httpHandler.RegisterRoutes(mux)

	grpcServer := grpc.NewServer()
	grpcService := grpcapi.NewServer(members, l)
	pb.RegisterClusterServiceServer(grpcServer, grpcService)
	reflection.Register(grpcServer)

	httpServer := &nethttp.Server{
		Addr:              cfg.APIAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &App{
		cfg:        cfg,
		l:          l,
		members:    members,
		engine:     engine,
		hc:         hc,
		httpServer: httpServer,
		grpcServer: grpcServer,
	}, nil
}

func (a *App) Start() error {
	a.l.Info("starting pheme",
		logger.Param("bind", a.cfg.BindAddr),
		logger.Param("api", a.cfg.APIAddr),
		logger.Param("zone", a.cfg.Zone),
	)

	a.engine.Start()

	if len(a.cfg.JoinAddrs) > 0 {
		if err := a.engine.Join(a.cfg.JoinAddrs); err != nil {
			a.l.Warn("failed to join cluster", logger.Error(err))
		}
	}

	a.hc.Start()
	go a.healthWatchLoop()

	go func() {
		a.l.Info("HTTP API listening", logger.Param("addr", a.cfg.APIAddr))
		if err := a.httpServer.ListenAndServe(); err != nil && err != nethttp.ErrServerClosed {
			a.l.Error("HTTP server error", logger.Error(err))
		}
	}()

	grpcAddr := incrementPort(a.cfg.APIAddr)
	lc := net.ListenConfig{}
	grpcLis, err := lc.Listen(context.Background(), "tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC on %s: %w", grpcAddr, err)
	}
	go func() {
		a.l.Info("gRPC API listening", logger.Param("addr", grpcAddr))
		if err := a.grpcServer.Serve(grpcLis); err != nil {
			a.l.Error("gRPC server error", logger.Error(err))
		}
	}()

	return nil
}

func (a *App) Stop(ctx context.Context) {
	a.l.Info("shutting down")

	if err := a.httpServer.Shutdown(ctx); err != nil {
		a.l.Warn("HTTP server shutdown error", logger.Error(err))
	}
	a.grpcServer.GracefulStop()
	a.engine.Stop()
	a.hc.Stop()

	a.l.Info("shutdown complete")
}

func (a *App) healthWatchLoop() {
	ticker := time.NewTicker(a.cfg.HealthcheckInterval)
	defer ticker.Stop()

	unhealthySince := time.Time{}
	deadThreshold := time.Duration(a.cfg.SuspicionMultiplier) * a.cfg.HealthcheckInterval

	for range ticker.C {
		if a.hc.Status() == healthcheck.StatusUnhealthy {
			self := a.members.GetSelf()
			if unhealthySince.IsZero() {
				unhealthySince = time.Now()
			}

			switch self.State {
			case pb.NodeState_NODE_STATE_ALIVE:
				a.l.Warn("local service unhealthy, marking self as suspect")
				a.members.SetState(self.ID, pb.NodeState_NODE_STATE_SUSPECT)
			case pb.NodeState_NODE_STATE_SUSPECT:
				if time.Since(unhealthySince) >= deadThreshold {
					a.l.Error("local service is continuously unhealthy, marking self as dead")
					a.members.SetState(self.ID, pb.NodeState_NODE_STATE_DEAD)
				}
			}
		} else {
			self := a.members.GetSelf()
			if self.State == pb.NodeState_NODE_STATE_SUSPECT || self.State == pb.NodeState_NODE_STATE_DEAD {
				a.l.Info("local service recovered, marking self as alive")
				a.members.SetState(self.ID, pb.NodeState_NODE_STATE_ALIVE)
			}
			unhealthySince = time.Time{}
		}
	}
}

func incrementPort(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		return addr
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", p+1))
}
