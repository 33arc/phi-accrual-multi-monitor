package phidetector

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
)

// PhiAccrualFailureDetector implements 'The Phi Accrual Failure Detector' by Hayashibara et al.
type PhiAccrualFailureDetector struct {
	threshold                      float64
	minStdDeviationMillis          float64
	acceptableHeartbeatPauseMillis int64
	heartbeatHistory               *HeartbeatHistory
	lastTimestampMillis            atomic.Int64
}

// NewPhiAccrualFailureDetector creates a new PhiAccrualFailureDetector
func NewPhiAccrualFailureDetector(threshold float64, maxSampleSize int, minStdDeviationMillis float64,
	acceptableHeartbeatPauseMillis int64, firstHeartbeatEstimateMillis int64) (*PhiAccrualFailureDetector, error) {
	if threshold <= 0 {
		return nil, fmt.Errorf("threshold must be > 0: %f", threshold)
	}
	if maxSampleSize <= 0 {
		return nil, fmt.Errorf("sample size must be > 0: %d", maxSampleSize)
	}
	if minStdDeviationMillis <= 0 {
		return nil, fmt.Errorf("minimum standard deviation must be > 0: %f", minStdDeviationMillis)
	}
	if acceptableHeartbeatPauseMillis < 0 {
		return nil, fmt.Errorf("acceptable heartbeat pause millis must be >= 0: %d", acceptableHeartbeatPauseMillis)
	}
	if firstHeartbeatEstimateMillis <= 0 {
		return nil, fmt.Errorf("first heartbeat value must be > 0: %d", firstHeartbeatEstimateMillis)
	}

	stdDeviationMillis := firstHeartbeatEstimateMillis / 4
	heartbeatHistory := NewHeartbeatHistory(maxSampleSize)
	heartbeatHistory.Add(firstHeartbeatEstimateMillis - stdDeviationMillis)
	heartbeatHistory.Add(firstHeartbeatEstimateMillis + stdDeviationMillis)

	detector := &PhiAccrualFailureDetector{
		threshold:                      threshold,
		minStdDeviationMillis:          minStdDeviationMillis,
		acceptableHeartbeatPauseMillis: acceptableHeartbeatPauseMillis,
		heartbeatHistory:               heartbeatHistory,
	}
	detector.lastTimestampMillis.Store(0)

	return detector, nil
}

func (p *PhiAccrualFailureDetector) ensureValidStdDeviation(stdDeviationMillis float64) float64 {
	return math.Max(stdDeviationMillis, p.minStdDeviationMillis)
}

func (p *PhiAccrualFailureDetector) Phi(timestampMillis int64) float64 {
	lastTimestampMillis := p.lastTimestampMillis.Load()
	if lastTimestampMillis == 0 {
		return 0.0
	}

	timeDiffMillis := timestampMillis - lastTimestampMillis
	meanMillis := p.heartbeatHistory.Mean() + float64(p.acceptableHeartbeatPauseMillis)
	stdDeviationMillis := p.ensureValidStdDeviation(p.heartbeatHistory.StdDeviation())

	y := (float64(timeDiffMillis) - meanMillis) / stdDeviationMillis
	e := math.Exp(-y * (1.5976 + 0.070566*y*y))
	if float64(timeDiffMillis) > meanMillis {
		return -math.Log10(e / (1.0 + e))
	} else {
		return -math.Log10(1.0 - 1.0/(1.0+e))
	}
}

func (p *PhiAccrualFailureDetector) IsAvailable(timestampMillis int64) bool {
	return p.Phi(timestampMillis) < p.threshold
}

func (p *PhiAccrualFailureDetector) Heartbeat(timestampMillis int64) {
	lastTimestampMillis := p.lastTimestampMillis.Swap(timestampMillis)
	if lastTimestampMillis != 0 {
		interval := timestampMillis - lastTimestampMillis
		if p.IsAvailable(timestampMillis) {
			p.heartbeatHistory.Add(interval)
		}
	}
}

// HeartbeatHistory represents the history of heartbeats
type HeartbeatHistory struct {
	maxSampleSize      int
	intervals          []int64
	intervalSum        int64
	squaredIntervalSum int64
	mu                 sync.RWMutex
}

func NewHeartbeatHistory(maxSampleSize int) *HeartbeatHistory {
	if maxSampleSize < 1 {
		panic(fmt.Sprintf("maxSampleSize must be >= 1, got %d", maxSampleSize))
	}
	return &HeartbeatHistory{
		maxSampleSize: maxSampleSize,
		intervals:     make([]int64, 0, maxSampleSize),
	}
}

func (h *HeartbeatHistory) Mean() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.intervals) == 0 {
		return 0
	}
	return float64(h.intervalSum) / float64(len(h.intervals))
}

func (h *HeartbeatHistory) Variance() float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.intervals) == 0 {
		return 0
	}
	mean := float64(h.intervalSum) / float64(len(h.intervals))
	return (float64(h.squaredIntervalSum) / float64(len(h.intervals))) - (mean * mean)
}

func (h *HeartbeatHistory) StdDeviation() float64 {
	return math.Sqrt(h.Variance())
}

func (h *HeartbeatHistory) Add(interval int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.intervals) >= h.maxSampleSize {
		dropped := h.intervals[0]
		h.intervals = h.intervals[1:]
		h.intervalSum -= dropped
		h.squaredIntervalSum -= dropped * dropped
	}
	h.intervals = append(h.intervals, interval)
	h.intervalSum += interval
	h.squaredIntervalSum += interval * interval
}

// Builder for PhiAccrualFailureDetector
type Builder struct {
	threshold                      float64
	maxSampleSize                  int
	minStdDeviationMillis          float64
	acceptableHeartbeatPauseMillis int64
	firstHeartbeatEstimateMillis   int64
}

func NewBuilder() *Builder {
	return &Builder{
		threshold:                      16.0,
		maxSampleSize:                  200,
		minStdDeviationMillis:          500,
		acceptableHeartbeatPauseMillis: 0,
		firstHeartbeatEstimateMillis:   500,
	}
}

func (b *Builder) SetThreshold(threshold float64) *Builder {
	b.threshold = threshold
	return b
}

func (b *Builder) SetMaxSampleSize(maxSampleSize int) *Builder {
	b.maxSampleSize = maxSampleSize
	return b
}

func (b *Builder) SetMinStdDeviationMillis(minStdDeviationMillis float64) *Builder {
	b.minStdDeviationMillis = minStdDeviationMillis
	return b
}

func (b *Builder) SetAcceptableHeartbeatPauseMillis(acceptableHeartbeatPauseMillis int64) *Builder {
	b.acceptableHeartbeatPauseMillis = acceptableHeartbeatPauseMillis
	return b
}

func (b *Builder) SetFirstHeartbeatEstimateMillis(firstHeartbeatEstimateMillis int64) *Builder {
	b.firstHeartbeatEstimateMillis = firstHeartbeatEstimateMillis
	return b
}

func (b *Builder) Build() (*PhiAccrualFailureDetector, error) {
	return NewPhiAccrualFailureDetector(b.threshold, b.maxSampleSize, b.minStdDeviationMillis,
		b.acceptableHeartbeatPauseMillis, b.firstHeartbeatEstimateMillis)
}
