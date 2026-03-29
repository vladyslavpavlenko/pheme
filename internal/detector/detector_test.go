package detector_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vladyslavpavlenko/pheme/internal/detector"
)

func TestRTTWindow(t *testing.T) {
	w := detector.NewRTTWindow(5)

	require.Equal(t, 0, w.Count())

	w.Add(10 * time.Millisecond)
	w.Add(20 * time.Millisecond)
	w.Add(30 * time.Millisecond)

	assert.Equal(t, 20*time.Millisecond, w.Mean())
	assert.Equal(t, 3, w.Count())
}

func TestRTTWindowOverflow(t *testing.T) {
	w := detector.NewRTTWindow(3)

	w.Add(10 * time.Millisecond)
	w.Add(20 * time.Millisecond)
	w.Add(30 * time.Millisecond)
	w.Add(40 * time.Millisecond)

	assert.Equal(t, 3, w.Count())
	assert.Equal(t, 30*time.Millisecond, w.Mean())
}

func TestAdaptiveSuspicionTimeout(t *testing.T) {
	d := detector.New(50, 4.0)

	assert.Equal(t, 1*time.Second, d.SuspicionTimeout("unknown-node"))

	for i := 0; i < 10; i++ {
		d.RecordRTT("fast-node", 5*time.Millisecond)
	}
	fastTimeout := d.SuspicionTimeout("fast-node")
	assert.LessOrEqual(t, fastTimeout, 500*time.Millisecond)

	for i := 0; i < 10; i++ {
		d.RecordRTT("slow-node", 100*time.Millisecond)
	}
	slowTimeout := d.SuspicionTimeout("slow-node")
	assert.GreaterOrEqual(t, slowTimeout, fastTimeout)
}

func TestPingTimeout(t *testing.T) {
	d := detector.New(50, 4.0)

	assert.Equal(t, 500*time.Millisecond, d.PingTimeout("unknown"))

	for i := 0; i < 10; i++ {
		d.RecordRTT("node-a", 10*time.Millisecond)
	}
	assert.LessOrEqual(t, d.PingTimeout("node-a"), 200*time.Millisecond)
}
