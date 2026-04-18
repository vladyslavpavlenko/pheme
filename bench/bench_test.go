package bench_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/vladyslavpavlenko/pheme/bench"
)

func TestComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping comparison benchmark in short mode")
	}

	sizes := []int{5, 10, 50, 100, 500}

	bench.CheckFDLimit(uint64(sizes[len(sizes)-1]) * 3)

	var results []*bench.Result

	for _, n := range sizes {
		t.Run("", func(t *testing.T) {
			pr, err := bench.RunPheme(n)
			if err != nil {
				t.Fatalf("pheme %d nodes: %v", n, err)
			}
			results = append(results, pr)

			runtime.GC()
			time.Sleep(3 * time.Second)

			mr, err := bench.RunMemberlist(n)
			if err != nil {
				t.Fatalf("memberlist %d nodes: %v", n, err)
			}
			results = append(results, mr)

			runtime.GC()
			time.Sleep(3 * time.Second)
		})
	}

	bench.PrintResults(results)
}
