package bench

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

type Result struct {
	System           string
	Nodes            int
	Convergence      time.Duration
	FailureDetect    time.Duration
	BandwidthPerNode float64 // bytes/sec
}

func CheckFDLimit(needed uint64) {
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		fmt.Fprintf(os.Stderr, "[bench] warning: could not check fd limit: %v\n", err)
		return
	}
	if rlimit.Cur < needed {
		fmt.Fprintf(os.Stderr, "[bench] warning: fd limit is %d, need ~%d. Run: ulimit -n %d\n",
			rlimit.Cur, needed, needed)
	}
}

func PrintResults(results []*Result) {
	fmt.Println()
	fmt.Printf("%-12s %-7s %-14s %-18s %-16s\n",
		"System", "Nodes", "Convergence", "Failure Detect", "Bandwidth/node")
	fmt.Println("-----------  ------  ------------  ----------------  --------------")
	for _, r := range results {
		bw := fmt.Sprintf("%.0f B/s", r.BandwidthPerNode)
		if r.BandwidthPerNode > 1024 {
			bw = fmt.Sprintf("%.1f KB/s", r.BandwidthPerNode/1024)
		}
		fmt.Printf("%-12s %-7d %-14s %-18s %-16s\n",
			r.System,
			r.Nodes,
			r.Convergence.Round(time.Millisecond),
			r.FailureDetect.Round(time.Millisecond),
			bw,
		)
	}
	fmt.Println()
}
