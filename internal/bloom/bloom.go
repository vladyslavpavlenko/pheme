// Package bloom implements a rotating pair of bloom filters so membership tests cover a recent time window.
package bloom

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"
)

// Filter holds two bit sets: writes go to the current set while lookups consider both during rotation.
type Filter struct {
	mu       sync.RWMutex
	numHash  uint
	numBits  uint
	current  []uint64
	previous []uint64

	rotationInterval time.Duration
	stopCh           chan struct{}
}

// New allocates a filter sized for expectedItems and fpRate, rotating bit sets every rotationInterval.
func New(expectedItems uint, fpRate float64, rotationInterval time.Duration) *Filter {
	numBits := optimalBits(expectedItems, fpRate)
	numHash := optimalHashFuncs(numBits, expectedItems)

	words := (numBits + 63) / 64

	f := &Filter{
		numHash:          numHash,
		numBits:          numBits,
		current:          make([]uint64, words),
		previous:         make([]uint64, words),
		rotationInterval: rotationInterval,
		stopCh:           make(chan struct{}),
	}
	return f
}

// Start begins swapping current and previous filters on rotationInterval in a background goroutine.
func (f *Filter) Start() {
	go f.rotateLoop()
}

// Stop ends the rotation loop; safe to call only once after Start.
func (f *Filter) Stop() {
	close(f.stopCh)
}

func (f *Filter) rotateLoop() {
	ticker := time.NewTicker(f.rotationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			f.rotate()
		case <-f.stopCh:
			return
		}
	}
}

func (f *Filter) rotate() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.previous = f.current
	f.current = make([]uint64, len(f.previous))
}

// Add inserts data into the current bit set using double hashing across numHash positions.
func (f *Filter) Add(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	h1, h2 := hash(data)
	for i := uint(0); i < f.numHash; i++ {
		pos := (h1 + uint64(i)*h2) % uint64(f.numBits)
		f.current[pos/64] |= 1 << (pos % 64)
	}
}

// Contains checks both the current and previous bloom filters.
func (f *Filter) Contains(data []byte) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	h1, h2 := hash(data)
	for i := uint(0); i < f.numHash; i++ {
		pos := (h1 + uint64(i)*h2) % uint64(f.numBits)
		word := pos / 64
		bit := pos % 64

		inCurrent := (f.current[word] >> bit) & 1
		inPrevious := (f.previous[word] >> bit) & 1

		if inCurrent == 0 && inPrevious == 0 {
			return false
		}
	}
	return true
}

// AddIfNotPresent checks and adds atomically, returning true if the item was new.
func (f *Filter) AddIfNotPresent(data []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	h1, h2 := hash(data)
	positions := make([]uint64, f.numHash)
	allSet := true

	for i := uint(0); i < f.numHash; i++ {
		pos := (h1 + uint64(i)*h2) % uint64(f.numBits)
		positions[i] = pos

		word := pos / 64
		bit := pos % 64

		inCurrent := (f.current[word] >> bit) & 1
		inPrevious := (f.previous[word] >> bit) & 1

		if inCurrent == 0 && inPrevious == 0 {
			allSet = false
		}
	}

	for _, pos := range positions {
		f.current[pos/64] |= 1 << (pos % 64)
	}

	return !allSet
}

// Fingerprint hashes nodeID, state, and version into 32 bytes suitable as filter keys.
func Fingerprint(nodeID string, state int32, version uint64) []byte {
	s := fmt.Sprintf("%s:%d:%d", nodeID, state, version)
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func hash(data []byte) (uint64, uint64) {
	h := sha256.Sum256(data)
	h1 := binary.BigEndian.Uint64(h[0:8])
	h2 := binary.BigEndian.Uint64(h[8:16])
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

func optimalBits(n uint, p float64) uint {
	m := -float64(n) * math.Log(p) / (math.Log(2) * math.Log(2))
	return uint(math.Ceil(m))
}

func optimalHashFuncs(m, n uint) uint {
	k := float64(m) / float64(n) * math.Log(2)
	return uint(math.Ceil(k))
}
