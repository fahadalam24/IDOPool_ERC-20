package core

import (
	"math"
	"sync"
)

// ConflictResolver handles advanced conflict detection and resolution
type ConflictResolver struct {
	mu            sync.RWMutex
	stateEntropy  map[string]float64 // Track entropy per state key
	conflictLog   map[string][]StateConflict
	orchestrator  *ConsistencyOrchestrator
}

// StateConflict represents a detected conflict in state
type StateConflict struct {
	Key           string
	Values        [][]byte
	VectorClocks  []VectorClock
	EntropyScore  float64
	ResolutionProb float64
}

// NewConflictResolver creates a new instance of ConflictResolver
func NewConflictResolver(orchestrator *ConsistencyOrchestrator) *ConflictResolver {
	return &ConflictResolver{
		stateEntropy:  make(map[string]float64),
		conflictLog:   make(map[string][]StateConflict),
		orchestrator:  orchestrator,
	}
}

// calculateStateEntropy computes Shannon entropy for state changes
func (cr *ConflictResolver) calculateStateEntropy(values [][]byte) float64 {
	if len(values) == 0 {
		return 0.0
	}

	// Count frequency of each value
	freqMap := make(map[string]int)
	total := len(values)

	for _, val := range values {
		freqMap[string(val)]++
	}

	// Calculate entropy using Shannon's formula: -Σ(p * log2(p))
	var entropy float64
	for _, freq := range freqMap {
		p := float64(freq) / float64(total)
		entropy -= p * math.Log2(p)
	}

	return entropy
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
		}
	}

	if !hasConflict {
		return nil
	}

	// Calculate resolution probability based on entropy
	maxEntropy := math.Log2(float64(len(values))) // Maximum possible entropy
	resolutionProb := 1.0 - (entropy / maxEntropy)

	conflict := &StateConflict{
		Key:           key,
		Values:        values,
		VectorClocks:  clocks,
		EntropyScore:  entropy,
		ResolutionProb: resolutionProb,
	}

	// Log the conflict
	cr.conflictLog[key] = append(cr.conflictLog[key], *conflict)

	return conflict
}

// ResolveConflict uses probabilistic resolution based on entropy and vector clocks
func (cr *ConflictResolver) ResolveConflict(conflict *StateConflict) ([]byte, VectorClock) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if conflict == nil || len(conflict.Values) == 0 {
		return nil, nil
	}

	// If resolution probability is high, use vector clock precedence
	if conflict.ResolutionProb > 0.8 {
		// Find the latest value according to vector clocks
		latestIdx := 0
		for i := 1; i < len(conflict.VectorClocks); i++ {
			if cr.orchestrator.Compare(conflict.VectorClocks[i], conflict.VectorClocks[latestIdx]) > 0 {
				latestIdx = i
			}
		}
		return conflict.Values[latestIdx], conflict.VectorClocks[latestIdx]
	}

	// For lower resolution probability, use entropy-based merge strategy
	// Select the value that minimizes entropy increase
	minEntropyIncrease := math.MaxFloat64
	selectedIdx := 0

	for i, value := range conflict.Values {
		// Calculate hypothetical entropy if this value is chosen
		tempValues := append([][]byte{}, conflict.Values...)
		tempValues[0] = value // Replace first value with current candidate
		newEntropy := cr.calculateStateEntropy(tempValues)
		entropyIncrease := newEntropy - conflict.EntropyScore

		if entropyIncrease < minEntropyIncrease {
			minEntropyIncrease = entropyIncrease
			selectedIdx = i
		}
	}

	// Merge vector clocks to maintain causal history
	mergedClock := conflict.VectorClocks[0]
	for i := 1; i < len(conflict.VectorClocks); i++ {
		mergedClock = cr.orchestrator.MergeVectorClocks(mergedClock, conflict.VectorClocks[i])
	}

	return conflict.Values[selectedIdx], mergedClock
}

// GetConflictHistory returns the conflict history for a key
func (cr *ConflictResolver) GetConflictHistory(key string) []StateConflict {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.conflictLog[key]
}

// GetStateEntropy returns the current entropy for a key
func (cr *ConflictResolver) GetStateEntropy(key string) float64 {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.stateEntropy[key]
}