// Package detector adapts suspicion and ping timeouts from per-node RTT samples.
package detector

import (
	"math"
	"sync"
	"time"
)

// RTTWindow stores recent round-trip times in a fixed-capacity ring buffer.
type RTTWindow struct {
	samples    []time.Duration
	pos        int
	count      int
	windowSize int
}

// NewRTTWindow allocates a buffer that retains at most size samples.
func NewRTTWindow(size int) *RTTWindow {
	return &RTTWindow{
		samples:    make([]time.Duration, size),
		windowSize: size,
	}
}

// Add records one RTT, overwriting the oldest entry once the buffer is full.
func (w *RTTWindow) Add(rtt time.Duration) {
	w.samples[w.pos] = rtt
	w.pos = (w.pos + 1) % w.windowSize
	if w.count < w.windowSize {
		w.count++
	}
}

// Mean returns the average of stored samples, or zero if empty.
func (w *RTTWindow) Mean() time.Duration {
	if w.count == 0 {
		return 0
	}
	var sum time.Duration
	for i := 0; i < w.count; i++ {
		sum += w.samples[i]
	}
	return sum / time.Duration(w.count)
}

// StdDev returns the population standard deviation, or zero if fewer than two samples.
func (w *RTTWindow) StdDev() time.Duration {
	if w.count < 2 {
		return 0
	}
	mean := w.Mean()
	var sumSq float64
	for i := 0; i < w.count; i++ {
		diff := float64(w.samples[i] - mean)
		sumSq += diff * diff
	}
	return time.Duration(math.Sqrt(sumSq / float64(w.count)))
}

// Count is how many samples are currently in the window (up to capacity).
func (w *RTTWindow) Count() int {
	return w.count
}

// Detector keeps per-node RTT windows to derive timeout durations.
type Detector struct {
	mu         sync.RWMutex
	windows    map[string]*RTTWindow
	windowSize int
	multiplier float64

	defaultTimeout time.Duration
}

// New builds a detector; multiplier scales RTT variance in SuspicionTimeout.
func New(windowSize int, multiplier float64) *Detector {
	return &Detector{
		windows:        make(map[string]*RTTWindow),
		windowSize:     windowSize,
		multiplier:     multiplier,
		defaultTimeout: 1 * time.Second,
	}
}

// RecordRTT appends an observation for nodeID, creating a window if needed.
func (d *Detector) RecordRTT(nodeID string, rtt time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	w, ok := d.windows[nodeID]
	if !ok {
		w = NewRTTWindow(d.windowSize)
		d.windows[nodeID] = w
	}
	w.Add(rtt)
}

// SuspicionTimeout computes: mean(RTT) + multiplier * stddev(RTT)
func (d *Detector) SuspicionTimeout(nodeID string) time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	w, ok := d.windows[nodeID]
	if !ok || w.Count() < 3 {
		return d.defaultTimeout
	}

	mean := w.Mean()
	stddev := w.StdDev()
	timeout := mean + time.Duration(d.multiplier*float64(stddev))

	minTimeout := 100 * time.Millisecond
	if timeout < minTimeout {
		return minTimeout
	}
	return timeout
}

// PingTimeout returns a shorter timeout for direct pings.
func (d *Detector) PingTimeout(nodeID string) time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	w, ok := d.windows[nodeID]
	if !ok || w.Count() < 3 {
		return 500 * time.Millisecond
	}

	mean := w.Mean()
	stddev := w.StdDev()
	timeout := mean + time.Duration(2.0*float64(stddev))

	minTimeout := 50 * time.Millisecond
	if timeout < minTimeout {
		return minTimeout
	}
	return timeout
}

// Remove discards all RTT history for nodeID.
func (d *Detector) Remove(nodeID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.windows, nodeID)
}
