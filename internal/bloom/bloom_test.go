package bloom_test

import (
	"fmt"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vladyslavpavlenko/pheme/internal/bloom"
)

func TestAddAndContains(t *testing.T) {
	f := bloom.New(1000, 0.01, time.Minute)

	data := []byte("test-item-1")
	assert.False(t, f.Contains(data), "empty filter should not contain item")

	f.Add(data)
	assert.True(t, f.Contains(data), "item should be found after Add")
}

func TestAddIfNotPresent(t *testing.T) {
	f := bloom.New(1000, 0.01, time.Minute)

	data := []byte("unique-item")

	assert.True(t, f.AddIfNotPresent(data), "first insertion should return true")
	assert.False(t, f.AddIfNotPresent(data), "second insertion should return false")
}

func TestRotation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := bloom.New(1000, 0.01, time.Minute)
		f.Start()
		defer f.Stop()

		data := []byte("pre-rotation")
		f.Add(data)

		time.Sleep(time.Minute)
		synctest.Wait()
		assert.True(t, f.Contains(data), "item should survive one rotation (in previous filter)")

		time.Sleep(time.Minute)
		synctest.Wait()
		assert.False(t, f.Contains(data), "item should be gone after two rotations")
	})
}

func TestFingerprint(t *testing.T) {
	fp1 := bloom.Fingerprint("node-1", 0, 1)
	fp2 := bloom.Fingerprint("node-1", 0, 2)
	fp3 := bloom.Fingerprint("node-1", 0, 1)

	assert.NotEqual(t, fp1, fp2, "different versions should produce different fingerprints")
	assert.Equal(t, fp1, fp3, "same inputs should produce identical fingerprints")
}

func TestFalsePositiveRate(t *testing.T) {
	n := uint(10000)
	f := bloom.New(n, 0.01, time.Minute)

	for i := uint(0); i < n; i++ {
		f.Add([]byte(fmt.Sprintf("item-%d", i)))
	}

	falsePositives := 0
	testCount := 100000
	for i := 0; i < testCount; i++ {
		if f.Contains([]byte(fmt.Sprintf("nonexistent-%d", i))) {
			falsePositives++
		}
	}

	fpRate := float64(falsePositives) / float64(testCount)
	t.Logf("False positive rate: %.4f (expected ~0.01)", fpRate)

	require.LessOrEqual(t, fpRate, 0.02, "false positive rate should not exceed 2%%")
}
