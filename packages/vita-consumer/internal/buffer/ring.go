package buffer

import (
	"sync"
	"time"
)

type Metric struct {
	Time       time.Time
	ResourceID int64
	Type       string
	Value      float64
}

// RingBuffer is a simplified circular buffer or slice-based buffer
type RingBuffer struct {
	mu      sync.RWMutex
	metrics []Metric
	maxSize int
}

func NewRingBuffer(maxSize int) *RingBuffer {
	return &RingBuffer{
		metrics: make([]Metric, 0, maxSize),
		maxSize: maxSize,
	}
}

func (rb *RingBuffer) Add(m Metric) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Simple slice append for now.
	// In a real circular buffer we would overwrite.
	// Here we just append and rely on Flush to clear.
	if len(rb.metrics) >= rb.maxSize {
		// Drop oldest? or Block?
		// For efficiency, let's just drop for now or expand.
		// Dropping is safer for memory.
		return
	}
	rb.metrics = append(rb.metrics, m)
}

func (rb *RingBuffer) Flush() []Metric {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	// Swap buffer
	old := rb.metrics
	rb.metrics = make([]Metric, 0, rb.maxSize)
	return old
}

func (rb *RingBuffer) ReadAll() []Metric {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	// Copy to avoid race conditions during iteration by caller
	result := make([]Metric, len(rb.metrics))
	copy(result, rb.metrics)
	return result
}
