package core

import (
	"log"
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

// GetNetworkStats returns network statistics for a node
func (co *ConsistencyOrchestrator) GetNetworkStats(nodeID string) *NetworkStats {
	co.mu.RLock()
	defer co.mu.RUnlock()
	return co.networkStats[nodeID]
}

// UpdateNetworkStats updates network statistics for a node
func (co *ConsistencyOrchestrator) UpdateNetworkStats(nodeID string, latency float64, packetLoss float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	if co.networkStats == nil {
		co.networkStats = make(map[string]*NetworkStats)
	}

	stats, exists := co.networkStats[nodeID]
	if !exists {
		log.Printf("Creating new network stats for node %s", nodeID)
		stats = &NetworkStats{}
		co.networkStats[nodeID] = stats
	}

	// Update with exponential moving average
	alpha := 0.2 // Smoothing factor
	stats.AvgLatency = (alpha * latency) + ((1 - alpha) * stats.AvgLatency)
	stats.PacketLoss = (alpha * packetLoss) + ((1 - alpha) * stats.PacketLoss)
	
	// Calculate partition probability based on latency and packet loss
	stats.PartitionProb = math.Min(1.0, (stats.PacketLoss*0.7 + (stats.AvgLatency/1000.0)*0.3))
	stats.LastUpdated = time.Now()

	// log.Printf("Updated stats for node %s: latency=%.2fms, packetLoss=%.2f%%, partitionProb=%.2f%%", 
    //           nodeID, stats.AvgLatency, stats.PacketLoss*100, stats.PartitionProb*100)

	// Adjust consistency mechanisms based on new stats
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
		oldLevel := co.currentLevel

		// Adjust consistency level based on partition probability
		switch {
		case avgPartitionProb < 0.1:
			co.currentLevel = StrongConsistency
		case avgPartitionProb < 0.3:
			co.currentLevel = CausalConsistency
		default:
			co.currentLevel = EventualConsistency
		}

		if oldLevel != co.currentLevel {
			log.Printf("Consistency level changed from %v to %v (avgPartitionProb=%.2f%%)", 
                      oldLevel, co.currentLevel, avgPartitionProb*100)
		}
	}
}

// adjustTimeoutAndRetry adjusts timeout and retry parameters based on network conditions
func (co *ConsistencyOrchestrator) adjustTimeoutAndRetry() {
	var maxLatency float64
	var avgPacketLoss float64
	nodeCount := 0

	for _, stats := range co.networkStats {
		if stats.AvgLatency > maxLatency {
			maxLatency = stats.AvgLatency
		}
		avgPacketLoss += stats.PacketLoss
		nodeCount++
	}

	if nodeCount > 0 {
		avgPacketLoss /= float64(nodeCount)
	}

	// Set timeout to 3x the maximum observed latency, with minimum of 1 second
	co.timeoutMS = int64(math.Max(1000, 3*maxLatency))

	// Adjust retry attempts based on packet loss
	// More retries when packet loss is high, but cap at reasonable maximum
	co.retryAttempts = int(math.Max(3, math.Min(10, 3/math.Max(0.1, 1-avgPacketLoss))))
}

// UpdateVectorClock updates the vector clock for a node
func (co *ConsistencyOrchestrator) UpdateVectorClock(nodeID string) {
	co.mu.Lock()
	defer co.mu.Unlock()

	if co.vectorClocks == nil {
		co.vectorClocks = make(map[string]VectorClock)
	}

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

// VerifyConsistency checks if an operation meets the current consistency requirements
func (co *ConsistencyOrchestrator) VerifyConsistency(nodeID string, operation string) bool {
	_ = operation
	co.mu.RLock()
	defer co.mu.RUnlock()

	result := false
	switch co.currentLevel {
	case StrongConsistency:
		result = co.verifyStrongConsistency(nodeID, operation)
		log.Printf("Strong consistency verification for node %s: %v", nodeID, result)
	case CausalConsistency:
		result = co.verifyCausalConsistency(nodeID, operation)
		log.Printf("Causal consistency verification for node %s: %v", nodeID, result)
	case EventualConsistency:
		result = true
		log.Printf("Eventual consistency assumed for node %s", nodeID)
	default:
		log.Printf("Unknown consistency level for node %s", nodeID)
		return false
	}
	return result
}

// verifyStrongConsistency implements strong consistency verification
func (co *ConsistencyOrchestrator) verifyStrongConsistency(nodeID string, operation string) bool {
	_ = operation
	// For strong consistency, we need majority acknowledgment
	totalNodes := len(co.networkStats)
	if totalNodes == 0 {
		return false
	}

	requiredNodes := (totalNodes / 2) + 1
	verifiedNodes := 0

	// In a real implementation, this would make network calls
	// For now, we simulate based on network conditions
	for peerID, stats := range co.networkStats {
		if peerID == nodeID {
			verifiedNodes++ // Count self
			continue
		}

		// Simulate verification based on network conditions
		if stats.PartitionProb < 0.5 && stats.PacketLoss < 0.3 {
			verifiedNodes++
		}

		if verifiedNodes >= requiredNodes {
			return true
		}
	}

	return false
}

// verifyCausalConsistency verifies causal consistency using vector clocks
func (co *ConsistencyOrchestrator) verifyCausalConsistency(nodeID string, operation string) bool {
	_ = operation
	clock, exists := co.vectorClocks[nodeID]
	if !exists {
		return false
	}

	// Verify that we have all causal dependencies
	for peer, count := range clock {
		if peer == nodeID {
			continue
		}

		peerClock, exists := co.vectorClocks[peer]
		if !exists || peerClock[peer] < count-1 {
			return false
		}
	}

	return true
}

// GetClockForNode returns the vector clock for a specific node
func (co *ConsistencyOrchestrator) GetClockForNode(nodeID string) VectorClock {
	co.mu.RLock()
	defer co.mu.RUnlock()
	if clock, exists := co.vectorClocks[nodeID]; exists {
		clockCopy := make(VectorClock)
		for k, v := range clock {
			clockCopy[k] = v
		}
		return clockCopy
	}
	return make(VectorClock)
}