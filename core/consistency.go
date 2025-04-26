package core

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// VectorClock represents a vector clock for causal consistency
type VectorClock map[string]uint64

// ConsistencyLevel represents different consistency requirements
type ConsistencyLevel int

const (
	StrongConsistency ConsistencyLevel = iota
	CausalConsistency
	EventualConsistency
)

// NetworkStats tracks network performance metrics
type NetworkStats struct {
	AvgLatency    float64
	PacketLoss    float64
	PartitionProb float64
	LastUpdated   time.Time
}

// ConsistencyOrchestrator manages adaptive consistency
type ConsistencyOrchestrator struct {
	mu             sync.RWMutex
	networkStats   map[string]*NetworkStats // Per-node network statistics
	vectorClocks   map[string]VectorClock  // Vector clocks for each node
	currentLevel   ConsistencyLevel
	timeoutMS     int64
	retryAttempts int
}

// NewConsistencyOrchestrator creates a new orchestrator instance
func NewConsistencyOrchestrator() *ConsistencyOrchestrator {
	return &ConsistencyOrchestrator{
		networkStats:   make(map[string]*NetworkStats),
		vectorClocks:   make(map[string]VectorClock),
		currentLevel:   EventualConsistency,
		timeoutMS:     1000, // Default 1 second
		retryAttempts: 3,    // Default 3 retries
	}
}

// UpdateNetworkStats updates network statistics for a node
func (co *ConsistencyOrchestrator) UpdateNetworkStats(nodeID string, latency float64, packetLoss float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	stats, exists := co.networkStats[nodeID]
	if !exists {
		stats = &NetworkStats{}
		co.networkStats[nodeID] = stats
	}

	// Exponential moving average for latency
	alpha := 0.2 // Smoothing factor
	stats.AvgLatency = alpha*latency + (1-alpha)*stats.AvgLatency
	stats.PacketLoss = packetLoss
	stats.LastUpdated = time.Now()

	// Calculate partition probability based on packet loss and latency
	latencyFactor := math.Min(1.0, stats.AvgLatency/1000.0) // Normalized to [0,1]
	stats.PartitionProb = (latencyFactor + stats.PacketLoss) / 2.0

	co.adjustConsistencyLevel()
	co.adjustTimeoutAndRetry()
}

// adjustConsistencyLevel dynamically adjusts consistency based on network conditions
func (co *ConsistencyOrchestrator) adjustConsistencyLevel() {
	var avgPartitionProb float64
	count := 0

	for _, stats := range co.networkStats {
		avgPartitionProb += stats.PartitionProb
		count++
	}

	if count > 0 {
		avgPartitionProb /= float64(count)

		// Adjust consistency level based on partition probability
		switch {
		case avgPartitionProb < 0.1:
			co.currentLevel = StrongConsistency
		case avgPartitionProb < 0.3:
			co.currentLevel = CausalConsistency
		default:
			co.currentLevel = EventualConsistency
		}
	}
}

// adjustTimeoutAndRetry adjusts timeout and retry parameters based on network conditions
func (co *ConsistencyOrchestrator) adjustTimeoutAndRetry() {
	var maxLatency float64

	for _, stats := range co.networkStats {
		if stats.AvgLatency > maxLatency {
			maxLatency = stats.AvgLatency
		}
	}

	// Set timeout to 3x the maximum observed latency
	co.timeoutMS = int64(math.Max(1000, 3*maxLatency))

	// Adjust retry attempts based on packet loss
	var avgPacketLoss float64
	count := 0
	for _, stats := range co.networkStats {
		avgPacketLoss += stats.PacketLoss
		count++
	}
	if count > 0 {
		avgPacketLoss /= float64(count)
		// More retries when packet loss is high
		co.retryAttempts = int(math.Max(3, math.Min(10, 3/math.Max(0.1, 1-avgPacketLoss))))
	}
}

// UpdateVectorClock updates the vector clock for a node
func (co *ConsistencyOrchestrator) UpdateVectorClock(nodeID string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	clock, exists := co.vectorClocks[nodeID]
	if !exists {
		clock = make(VectorClock)
		co.vectorClocks[nodeID] = clock
	}
	clock[nodeID]++
}

// MergeVectorClocks merges two vector clocks
func (co *ConsistencyOrchestrator) MergeVectorClocks(clock1, clock2 VectorClock) VectorClock {
	result := make(VectorClock)
	
	// Copy all values from clock1
	for k, v := range clock1 {
		result[k] = v
	}

	// Merge with clock2, taking the maximum values
	for k, v := range clock2 {
		if current, exists := result[k]; !exists || v > current {
			result[k] = v
		}
	}

	return result
}

// Compare returns -1 if clock1 < clock2, 0 if concurrent, 1 if clock1 > clock2
func (co *ConsistencyOrchestrator) Compare(clock1, clock2 VectorClock) int {
	less := false
	greater := false

	// Check each node's counter
	for node, count1 := range clock1 {
		count2, exists := clock2[node]
		if !exists {
			count2 = 0
		}
		if count1 < count2 {
			less = true
		}
		if count1 > count2 {
			greater = true
		}
	}

	// Check counters present in clock2 but not in clock1
	for node, count2 := range clock2 {
		if _, exists := clock1[node]; !exists && count2 > 0 {
			less = true
		}
	}

	if less && !greater {
		return -1
	}
	if greater && !less {
		return 1
	}
	return 0 // Concurrent events
}

// GetConsistencyLevel returns the current consistency level
func (co *ConsistencyOrchestrator) GetConsistencyLevel() ConsistencyLevel {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.currentLevel
}

// GetTimeout returns the current timeout value in milliseconds
func (co *ConsistencyOrchestrator) GetTimeout() int64 {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.timeoutMS
}

// GetRetryAttempts returns the current number of retry attempts
func (co *ConsistencyOrchestrator) GetRetryAttempts() int {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.retryAttempts
}

// TestCAP simulates different network conditions and tests adaptive consistency
func TestCAP() {
	// Initialize components
	orchestrator := NewConsistencyOrchestrator()
	resolver := NewConflictResolver(orchestrator)

	// Simulate good network conditions
	orchestrator.UpdateNetworkStats("node1", 50.0, 0.01)  // 50ms latency, 1% packet loss
	orchestrator.UpdateNetworkStats("node2", 60.0, 0.02)  // 60ms latency, 2% packet loss
	fmt.Printf("Good network conditions - Consistency Level: %v\n", orchestrator.GetConsistencyLevel())

	// Simulate degraded network
	orchestrator.UpdateNetworkStats("node1", 200.0, 0.15) // 200ms latency, 15% packet loss
	orchestrator.UpdateNetworkStats("node2", 250.0, 0.20) // 250ms latency, 20% packet loss
	fmt.Printf("Degraded network - Consistency Level: %v\n", orchestrator.GetConsistencyLevel())
	fmt.Printf("Adjusted timeout: %dms, Retries: %d\n", orchestrator.GetTimeout(), orchestrator.GetRetryAttempts())

	// Simulate conflict detection and resolution
	values := [][]byte{
		[]byte("value1"),
		[]byte("value2"),
		[]byte("value1"), // Duplicate to affect entropy
	}
	clocks := []VectorClock{
		{"node1": 1, "node2": 0},
		{"node1": 0, "node2": 1},
		{"node1": 1, "node2": 1},
	}

	conflict := resolver.DetectConflict("testKey", values, clocks)
	if conflict != nil {
		fmt.Printf("Conflict detected - Entropy Score: %.2f, Resolution Probability: %.2f\n", 
			conflict.EntropyScore, conflict.ResolutionProb)
		
		resolvedValue, resolvedClock := resolver.ResolveConflict(conflict)
		fmt.Printf("Resolved Value: %s\n", string(resolvedValue))
		fmt.Printf("Final Vector Clock: %v\n", resolvedClock)
	}
}