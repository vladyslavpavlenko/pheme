// Package healthcheck polls a co-located service's HTTP endpoint to
// track its availability and expose the result to the gossip layer.
package healthcheck

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/vladyslavpavlenko/pheme/internal/logger"
)

// Status classifies the outcome of the configured HTTP health probe.
type Status int

// Unknown: no completed probe yet. Healthy: last GET returned 2xx. Unhealthy: failure or non-2xx.
const (
	StatusUnknown Status = iota
	StatusHealthy
	StatusUnhealthy
)

func (s Status) String() string {
	switch s {
	case StatusHealthy:
		return "healthy"
	case StatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// Checker polls an HTTP URL on an interval and records whether the dependency is up.
type Checker struct {
	url      string
	interval time.Duration
	client   *http.Client
	l        *logger.Logger

	mu     sync.RWMutex
	status Status

	stopCh chan struct{}
}

// Option customizes a Checker when passed to New.
type Option func(*Checker)

// WithTransport replaces the default HTTP transport on the checker's client.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Checker) { c.client.Transport = rt }
}

// New creates a Checker that probes url every interval.
func New(url string, interval time.Duration, l *logger.Logger, opts ...Option) *Checker {
	c := &Checker{
		url:      url,
		interval: interval,
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
		status: StatusUnknown,
		stopCh: make(chan struct{}),
		l:      l,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Start begins periodic checks in a new goroutine; logs Fatal and exits if url is empty.
func (c *Checker) Start() {
	if c.url == "" {
		c.l.Fatal("healthcheck_url is not configured; a co-located service is required")
	}
	go c.loop()
}

// Stop closes the internal stop channel so the background loop returns.
func (c *Checker) Stop() {
	close(c.stopCh)
}

// Status returns the current probe classification; safe for concurrent readers.
func (c *Checker) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Checker) loop() {
	c.check()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.check()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Checker) check() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		c.transition(StatusUnhealthy)
		c.l.Warn("could not create a health check request", logger.Error(err))
		return
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.l.Debug("health check failed", logger.Param("url", c.url), logger.Error(err))
		c.transition(StatusUnhealthy)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.transition(StatusHealthy)
	} else {
		c.l.Debug("health check returned non-2xx", logger.Param("url", c.url), logger.Param("status", resp.StatusCode))
		c.transition(StatusUnhealthy)
	}
}

func (c *Checker) transition(s Status) {
	c.mu.Lock()
	prev := c.status
	c.status = s
	c.mu.Unlock()

	if prev != s {
		c.l.Info("health check status changed",
			logger.Param("from", prev.String()),
			logger.Param("to", s.String()),
		)
	}
}
