package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/vladyslavpavlenko/pheme/internal/app"
	"github.com/vladyslavpavlenko/pheme/internal/config"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
)

func main() {
	configPath := flag.String("config", "", "path to YAML config file")
	nodeID := flag.String("id", "", "node ID (overrides config)")
	bindAddr := flag.String("bind", "", "gossip bind address (overrides config)")
	apiAddr := flag.String("api", "", "HTTP API address (overrides config)")
	zone := flag.String("zone", "", "zone label (overrides config)")
	flag.Parse()

	l := logger.New(logger.LevelDebug)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		l.Fatal("could not load config", logger.Error(err))
	}
	applyOverrides(cfg, *nodeID, *bindAddr, *apiAddr, *zone)

	a, err := app.New(cfg, l)
	if err != nil {
		l.Fatal("could not initialize app", logger.Error(err))
	}

	if err := a.Start(); err != nil {
		l.Fatal("could not start app", logger.Error(err))
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.Stop(ctx)
}

func loadConfig(path string) (*config.Config, error) {
	if path != "" {
		return config.Load(path)
	}
	cfg := config.Default()
	if err := envconfig.Process("pheme", cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	return cfg, nil
}

func applyOverrides(cfg *config.Config, nodeID, bindAddr, apiAddr, zone string) {
	if nodeID != "" {
		cfg.NodeID = nodeID
	}
	if bindAddr != "" {
		cfg.BindAddr = bindAddr
	}
	if apiAddr != "" {
		cfg.APIAddr = apiAddr
	}
	if zone != "" {
		cfg.Zone = zone
	}
}
