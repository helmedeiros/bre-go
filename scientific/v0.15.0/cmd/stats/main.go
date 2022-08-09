// stats consumes two raw timings files (one nanosecond per line) and
// produces mean/median/p99/stddev for each plus a speedup ratio.
// Exits 0 unconditionally; the orchestrator interprets the result
// against pre-registered bars.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	var (
		baseline  = flag.String("baseline", "", "baseline timings file (nanoseconds per line)")
		candidate = flag.String("candidate", "", "candidate timings file (nanoseconds per line)")
		bar       = flag.Float64("bar", 0, "minimum median speedup (baseline/candidate) for PASS; 0 disables bar")
		labelA    = flag.String("labelA", "baseline", "label for baseline column")
		labelB    = flag.String("labelB", "candidate", "label for candidate column")
		report    = flag.String("report", "", "optional path to write a textual report")
	)
	flag.Parse()

	if *baseline == "" || *candidate == "" {
		fmt.Fprintln(os.Stderr, "stats: -baseline and -candidate are required")
		os.Exit(1)
	}

	a, err := readTimings(*baseline)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stats:", err)
		os.Exit(1)
	}
	b, err := readTimings(*candidate)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stats:", err)
		os.Exit(1)
	}

	sa := summarize(a)
	sb := summarize(b)

	speedupMedian := sa.median / sb.median
	speedupMean := sa.mean / sb.mean

	var out strings.Builder
	fmt.Fprintf(&out, "%-18s n=%d mean=%.3fms median=%.3fms p99=%.3fms stddev=%.3fms min=%.3fms max=%.3fms\n",
		*labelA, len(a), sa.mean/1e6, sa.median/1e6, sa.p99/1e6, sa.stddev/1e6, sa.min/1e6, sa.max/1e6)
	fmt.Fprintf(&out, "%-18s n=%d mean=%.3fms median=%.3fms p99=%.3fms stddev=%.3fms min=%.3fms max=%.3fms\n",
		*labelB, len(b), sb.mean/1e6, sb.median/1e6, sb.p99/1e6, sb.stddev/1e6, sb.min/1e6, sb.max/1e6)
	fmt.Fprintf(&out, "speedup (%s / %s): median=%.2fx mean=%.2fx\n", *labelA, *labelB, speedupMedian, speedupMean)

	verdict := "n/a"
	if *bar > 0 {
		if speedupMedian >= *bar {
			verdict = fmt.Sprintf("PASS (median %.2fx >= %.2fx)", speedupMedian, *bar)
		} else {
			verdict = fmt.Sprintf("FAIL (median %.2fx < %.2fx)", speedupMedian, *bar)
		}
		fmt.Fprintf(&out, "verdict: %s\n", verdict)
	}

	fmt.Print(out.String())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(out.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "stats: write report:", err)
			os.Exit(1)
		}
	}
}

func readTimings(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []float64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ns, err := strconv.ParseFloat(line, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, ns)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no measurements", path)
	}
	return out, nil
}

type summary struct {
	mean, median, p99, stddev, min, max float64
}

func summarize(xs []float64) summary {
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	var sum float64
	for _, x := range sorted {
		sum += x
	}
	mean := sum / float64(n)
	var ss float64
	for _, x := range sorted {
		d := x - mean
		ss += d * d
	}
	stddev := math.Sqrt(ss / float64(n))
	return summary{
		mean:   mean,
		median: percentile(sorted, 0.5),
		p99:    percentile(sorted, 0.99),
		stddev: stddev,
		min:    sorted[0],
		max:    sorted[n-1],
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
