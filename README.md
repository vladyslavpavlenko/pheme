<p align="center">
  <img src="assets/mascot.png" width="400" alt="Pheme">
</p>

# Pheme

Pheme is a sidecar gossip daemon that monitors a co-located service and
disseminates membership and health state across a cluster using a
SWIM-like protocol over UDP.

## Features

- **SWIM gossip** — protocol period, indirect probes, suspicion timeout.
- **Zone-aware peer selection** — biases gossip toward local-zone peers.
- **Adaptive failure detection** — per-node RTT tracking tunes suspicion and ping timeouts.
- **Bloom filter deduplication** — avoids re-processing recently seen state updates.
- **Local health checks** — polls a configurable HTTP endpoint; transitions the node through alive → suspect → dead.
- **HTTP/gRPC APIs** — query cluster status, list alive members, stream state changes.

## Quick Start

```bash
# build
task build

# run with example config
./pheme -config config.example.yaml

# or via Docker Compose (3-node cluster)
task docker:up
```

## Configuration

See [`config.example.yaml`](config.example.yaml) for all available options.
Settings can be overridden with environment variables prefixed with `PHEME_`
(e.g. `PHEME_BIND_ADDR`, `PHEME_JOIN_ADDRS`).

### Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 7946 | UDP | Gossip protocol |
| 7947 | TCP | HTTP API |
| 7948 | TCP | gRPC API |
