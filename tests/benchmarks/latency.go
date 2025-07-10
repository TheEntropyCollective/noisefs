package benchmarks

import (
	"fmt"
	"sort"
	"time"
)

// LatencyHistogram provides detailed latency analysis
type LatencyHistogram struct {
	buckets    []time.Duration
	counts     []int64
	totalCount int64
	sum        time.Duration
	min        time.Duration
	max        time.Duration
}

// NewLatencyHistogram creates a histogram with predefined buckets
func NewLatencyHistogram() *LatencyHistogram {
	// Create buckets for common latency ranges
	buckets := []time.Duration{
		10 * time.Microsecond,
		50 * time.Microsecond,
		100 * time.Microsecond,
		500 * time.Microsecond,
		1 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
		5 * time.Second,
		10 * time.Second,
	}

	return &LatencyHistogram{
		buckets: buckets,
		counts:  make([]int64, len(buckets)+1), // +1 for overflow bucket
		min:     time.Duration(1<<63 - 1),     // Max duration as initial min
		max:     0,
	}
}

// Record adds a latency measurement to the histogram
func (lh *LatencyHistogram) Record(latency time.Duration) {
	lh.totalCount++
	lh.sum += latency

	if latency < lh.min {
		lh.min = latency
	}
	if latency > lh.max {
		lh.max = latency
	}

	// Find the appropriate bucket
	bucketIndex := len(lh.buckets) // Default to overflow bucket
	for i, bucket := range lh.buckets {
		if latency <= bucket {
			bucketIndex = i
			break
		}
	}

	lh.counts[bucketIndex]++
}

// GetPercentile calculates the specified percentile
func (lh *LatencyHistogram) GetPercentile(percentile float64) time.Duration {
	if lh.totalCount == 0 {
		return 0
	}

	target := int64(float64(lh.totalCount) * percentile / 100.0)
	var cumulative int64

	for i, count := range lh.counts {
		cumulative += count
		if cumulative >= target {
			if i < len(lh.buckets) {
				return lh.buckets[i]
			}
			// Overflow bucket - return max
			return lh.max
		}
	}

	return lh.max
}

// GetMean returns the mean latency
func (lh *LatencyHistogram) GetMean() time.Duration {
	if lh.totalCount == 0 {
		return 0
	}
	return lh.sum / time.Duration(lh.totalCount)
}

// GetMin returns the minimum latency
func (lh *LatencyHistogram) GetMin() time.Duration {
	if lh.totalCount == 0 {
		return 0
	}
	return lh.min
}

// GetMax returns the maximum latency
func (lh *LatencyHistogram) GetMax() time.Duration {
	if lh.totalCount == 0 {
		return 0
	}
	return lh.max
}

// GetCount returns the total number of measurements
func (lh *LatencyHistogram) GetCount() int64 {
	return lh.totalCount
}

// PrintHistogram prints a text representation of the histogram
func (lh *LatencyHistogram) PrintHistogram() {
	fmt.Println("Latency Histogram:")
	fmt.Printf("Total samples: %d\n", lh.totalCount)
	fmt.Printf("Min: %v, Max: %v, Mean: %v\n", lh.min, lh.max, lh.GetMean())
	fmt.Printf("P50: %v, P95: %v, P99: %v, P99.9: %v\n",
		lh.GetPercentile(50), lh.GetPercentile(95),
		lh.GetPercentile(99), lh.GetPercentile(99.9))
	fmt.Println()

	for i, bucket := range lh.buckets {
		percentage := float64(lh.counts[i]) / float64(lh.totalCount) * 100
		bar := generateBar(percentage, 50)
		fmt.Printf("≤ %10v: %6d (%5.1f%%) %s\n", bucket, lh.counts[i], percentage, bar)
	}

	if lh.counts[len(lh.counts)-1] > 0 {
		percentage := float64(lh.counts[len(lh.counts)-1]) / float64(lh.totalCount) * 100
		bar := generateBar(percentage, 50)
		fmt.Printf("> %10v: %6d (%5.1f%%) %s\n", lh.buckets[len(lh.buckets)-1], lh.counts[len(lh.counts)-1], percentage, bar)
	}
}

// generateBar creates a simple text bar chart
func generateBar(percentage float64, maxWidth int) string {
	width := int(percentage / 100.0 * float64(maxWidth))
	if width > maxWidth {
		width = maxWidth
	}
	bar := ""
	for i := 0; i < width; i++ {
		bar += "█"
	}
	return bar
}

// LatencyAnalyzer provides comprehensive latency analysis
type LatencyAnalyzer struct {
	measurements []time.Duration
	sorted       bool
}

// NewLatencyAnalyzer creates a new latency analyzer
func NewLatencyAnalyzer() *LatencyAnalyzer {
	return &LatencyAnalyzer{
		measurements: make([]time.Duration, 0, 10000),
		sorted:       true,
	}
}

// Record adds a latency measurement
func (la *LatencyAnalyzer) Record(latency time.Duration) {
	la.measurements = append(la.measurements, latency)
	la.sorted = false
}

// ensureSorted sorts the measurements if needed
func (la *LatencyAnalyzer) ensureSorted() {
	if !la.sorted && len(la.measurements) > 0 {
		sort.Slice(la.measurements, func(i, j int) bool {
			return la.measurements[i] < la.measurements[j]
		})
		la.sorted = true
	}
}

// GetPercentile returns the exact percentile value
func (la *LatencyAnalyzer) GetPercentile(percentile float64) time.Duration {
	if len(la.measurements) == 0 {
		return 0
	}

	la.ensureSorted()

	index := float64(len(la.measurements)-1) * percentile / 100.0
	lower := int(index)
	upper := lower + 1

	if upper >= len(la.measurements) {
		return la.measurements[len(la.measurements)-1]
	}

	// Linear interpolation between the two closest values
	weight := index - float64(lower)
	return time.Duration(float64(la.measurements[lower])*(1-weight) + float64(la.measurements[upper])*weight)
}

// GetStats returns comprehensive statistics
func (la *LatencyAnalyzer) GetStats() LatencyStats {
	if len(la.measurements) == 0 {
		return LatencyStats{}
	}

	la.ensureSorted()

	var sum time.Duration
	for _, m := range la.measurements {
		sum += m
	}

	return LatencyStats{
		Count:  int64(len(la.measurements)),
		Min:    la.measurements[0],
		Max:    la.measurements[len(la.measurements)-1],
		Mean:   sum / time.Duration(len(la.measurements)),
		P50:    la.GetPercentile(50),
		P90:    la.GetPercentile(90),
		P95:    la.GetPercentile(95),
		P99:    la.GetPercentile(99),
		P999:   la.GetPercentile(99.9),
		P9999:  la.GetPercentile(99.99),
	}
}

// LatencyStats holds comprehensive latency statistics
type LatencyStats struct {
	Count int64         `json:"count"`
	Min   time.Duration `json:"min"`
	Max   time.Duration `json:"max"`
	Mean  time.Duration `json:"mean"`
	P50   time.Duration `json:"p50"`
	P90   time.Duration `json:"p90"`
	P95   time.Duration `json:"p95"`
	P99   time.Duration `json:"p99"`
	P999  time.Duration `json:"p999"`
	P9999 time.Duration `json:"p9999"`
}

// String returns a formatted string representation of the stats
func (ls LatencyStats) String() string {
	return fmt.Sprintf(
		"Count: %d, Min: %v, Max: %v, Mean: %v, P50: %v, P95: %v, P99: %v, P99.9: %v",
		ls.Count, ls.Min, ls.Max, ls.Mean, ls.P50, ls.P95, ls.P99, ls.P999,
	)
}

// GetOutliers identifies outliers using the IQR method
func (la *LatencyAnalyzer) GetOutliers() []time.Duration {
	if len(la.measurements) < 4 {
		return nil
	}

	la.ensureSorted()

	q1 := la.GetPercentile(25)
	q3 := la.GetPercentile(75)
	iqr := q3 - q1
	
	// Standard outlier detection: values beyond Q1 - 1.5*IQR or Q3 + 1.5*IQR
	lowerBound := q1 - time.Duration(float64(iqr)*1.5)
	upperBound := q3 + time.Duration(float64(iqr)*1.5)

	var outliers []time.Duration
	for _, measurement := range la.measurements {
		if measurement < lowerBound || measurement > upperBound {
			outliers = append(outliers, measurement)
		}
	}

	return outliers
}