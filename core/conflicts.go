package core

import (
	"bytes"
	"crypto/sha256"
	"math"
	"sort"
	"sync"
)

// StateConflict represents a detected conflict in state
type StateConflict struct {
	Key           string
	Values        [][]byte
	Clocks        []VectorClock
	EntropyScore  float64
	ResolutionProb float64
}

// ConflictResolver handles state conflict detection and resolution
type ConflictResolver struct {
	orchestrator   *ConsistencyOrchestrator
	stateEntropy  map[string]float64 // Track state entropy history
	mu            sync.RWMutex
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(orchestrator *ConsistencyOrchestrator) *ConflictResolver {
	return &ConflictResolver{
		orchestrator:  orchestrator,
		stateEntropy: make(map[string]float64),
	}
}

// DetectConflict checks for conflicts using entropy and vector clocks
func (cr *ConflictResolver) DetectConflict(key string, values [][]byte, clocks []VectorClock) *StateConflict {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Calculate entropy for the current state
	entropy := cr.calculateStateEntropy(values)
	
	// Track entropy history
	prevEntropy := cr.stateEntropy[key]
	cr.stateEntropy[key] = entropy

	// Detect conflict based on:
	// 1. Entropy increase (state divergence)
	// 2. Vector clock concurrency
	hasConflict := false
	
	// Check if entropy increased significantly
	if entropy > prevEntropy*1.2 { // 20% increase threshold
		hasConflict = true
	}

	// Check vector clock concurrency
	if len(clocks) > 1 {
		for i := 0; i < len(clocks)-1; i++ {
			for j := i + 1; j < len(clocks); j++ {
				if cr.orchestrator.Compare(clocks[i], clocks[j]) == 0 {
					hasConflict = true
					break
				}
			}
			if hasConflict {
				break
			}
		}
	}

	if !hasConflict {
		return nil
	}

	// Calculate resolution probability based on entropy and network conditions
	resolutionProb := cr.calculateResolutionProbability(entropy, len(values))

	return &StateConflict{
		Key:            key,
		Values:         values,
		Clocks:         clocks,
		EntropyScore:   entropy,
		ResolutionProb: resolutionProb,
	}
}

// ResolveConflict attempts to resolve a state conflict
func (cr *ConflictResolver) ResolveConflict(conflict *StateConflict) ([]byte, VectorClock) {
	if conflict == nil || len(conflict.Values) == 0 {
		return nil, nil
	}

	// Use multiple resolution strategies and weight their results
	strategies := []struct {
		name   string
		weight float64
		resolve func([][]byte, []VectorClock) ([]byte, VectorClock)
	}{
		{"timestamp", 0.4, cr.resolveByTimestamp},
		{"majority", 0.3, cr.resolveByMajority},
		{"entropy", 0.3, cr.resolveByEntropy},
	}

	type resolution struct {
		value []byte
		clock VectorClock
		weight float64
	}

	resolutions := make([]resolution, 0)

	// Apply each strategy
	for _, strategy := range strategies {
		value, clock := strategy.resolve(conflict.Values, conflict.Clocks)
		if value != nil {
			resolutions = append(resolutions, resolution{
				value:  value,
				clock:  clock,
				weight: strategy.weight,
			})
		}
	}

	if len(resolutions) == 0 {
		// Fallback to last-write-wins if no strategy produced a result
		return conflict.Values[len(conflict.Values)-1], 
		       conflict.Clocks[len(conflict.Clocks)-1]
	}

	// Merge resolutions based on weights
	var finalValue []byte
	var finalClock VectorClock
	maxWeight := 0.0

	for _, res := range resolutions {
		if res.weight > maxWeight {
			maxWeight = res.weight
			finalValue = res.value
			finalClock = res.clock
		}
	}

	return finalValue, finalClock
}

// calculateStateEntropy calculates Shannon entropy of state values
func (cr *ConflictResolver) calculateStateEntropy(values [][]byte) float64 {
	if len(values) <= 1 {
		return 0.0
	}

	// Count frequency of each value
	freq := make(map[string]int)
	total := len(values)

	for _, val := range values {
		hash := sha256.Sum256(val)
		freq[string(hash[:])]++
	}

	// Calculate Shannon entropy
	entropy := 0.0
	for _, count := range freq {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}

	return entropy
}

// calculateResolutionProbability estimates likelihood of successful resolution
func (cr *ConflictResolver) calculateResolutionProbability(entropy float64, valueCount int) float64 {
	// Base probability inversely proportional to entropy
	baseProb := 1.0 / (1.0 + entropy)

	// Adjust based on number of conflicting values
	valueAdjustment := 1.0 / math.Log2(float64(valueCount+1))

	// Network condition adjustment
	networkAdjustment := 1.0
	if cr.orchestrator != nil {
		stats := cr.orchestrator.GetNetworkStats("")
		if stats != nil {
			networkAdjustment = 1.0 - stats.PartitionProb
		}
	}

	return baseProb * valueAdjustment * networkAdjustment
}

// resolveByTimestamp resolves conflict by selecting the most recent value
func (cr *ConflictResolver) resolveByTimestamp(values [][]byte, clocks []VectorClock) ([]byte, VectorClock) {
	if len(values) == 0 || len(clocks) == 0 {
		return nil, nil
	}

	// Find the clock with highest total count (most recent)
	maxSum := uint64(0)
	maxIdx := 0

	for i, clock := range clocks {
		sum := uint64(0)
		for _, count := range clock {
			sum += count
		}
		if sum > maxSum {
			maxSum = sum
			maxIdx = i
		}
	}

	return values[maxIdx], clocks[maxIdx]
}

// resolveByMajority resolves conflict by selecting the most common value
func (cr *ConflictResolver) resolveByMajority(values [][]byte, clocks []VectorClock) ([]byte, VectorClock) {
	if len(values) == 0 {
		return nil, nil
	}

	// Count occurrences of each value
	type valueCount struct {
		value []byte
		clock VectorClock
		count int
	}

	counts := make(map[string]*valueCount)
	for i, val := range values {
		hash := sha256.Sum256(val)
		key := string(hash[:])
		
		if count, exists := counts[key]; exists {
			count.count++
		} else {
			counts[key] = &valueCount{
				value: val,
				clock: clocks[i],
				count: 1,
			}
		}
	}

	// Find value with highest count
	var maxCount *valueCount
	maxOccurrences := 0

	for _, count := range counts {
		if count.count > maxOccurrences {
			maxOccurrences = count.count
			maxCount = count
		}
	}

	if maxCount == nil {
		return nil, nil
	}

	return maxCount.value, maxCount.clock
}

// resolveByEntropy resolves conflict by selecting the value that minimizes entropy
func (cr *ConflictResolver) resolveByEntropy(values [][]byte, clocks []VectorClock) ([]byte, VectorClock) {
	if len(values) == 0 {
		return nil, nil
	}

	type candidate struct {
		value     []byte
		clock     VectorClock
		entropy   float64
	}

	candidates := make([]candidate, len(values))

	// Calculate entropy for each potential resolution
	for i, val := range values {
		// Create a test set without this value
		testSet := make([][]byte, 0, len(values)-1)
		testSet = append(testSet, values[:i]...)
		testSet = append(testSet, values[i+1:]...)

		// Add multiple copies of the candidate value
		testSet = append(testSet, val)
		testSet = append(testSet, val)

		entropy := cr.calculateStateEntropy(testSet)
		candidates[i] = candidate{
			value:   val,
			clock:   clocks[i],
			entropy: entropy,
		}
	}

	// Sort by entropy ascending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].entropy < candidates[j].entropy
	})

	// Return the value that would result in lowest entropy
	return candidates[0].value, candidates[0].clock
}

// CompareValues compares two values for equality
func (cr *ConflictResolver) CompareValues(a, b []byte) bool {
	return bytes.Equal(a, b)
}