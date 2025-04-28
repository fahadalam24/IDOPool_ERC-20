package core

import (
	"fmt"
	"sync"
	"time"
)

var (
	// ShardRegistry maintains all active shards
	ShardRegistry = make(map[string]*Shard)
	shardMutex   sync.RWMutex
)

// Shard represents a blockchain shard with its own state
type Shard struct {
	ID            string
	ParentID      string
	ChildrenIDs   []string
	MerkleRoot    []byte
	StateData     map[string][]byte
	TxCount       int
	LastActivity  int64
	mutex         sync.RWMutex
	PedersenCommit *PedersenCommitment // Add Pedersen commitment field
}

// CreateShard creates a new shard with the given ID and optional parent ID
func CreateShard(id string, parentID string) *Shard {
	shardMutex.Lock()
	defer shardMutex.Unlock()

	shard := &Shard{
		ID:           id,
		ParentID:     parentID,
		ChildrenIDs:  make([]string, 0),
		StateData:    make(map[string][]byte),
		TxCount:      0,
		LastActivity: time.Now().Unix(),
		PedersenCommit: &PedersenCommitment{}, // Initialize PedersenCommitment
	}

	ShardRegistry[id] = shard
	return shard
}

// GetState retrieves a value from the shard's state
func (s *Shard) GetState(key string) ([]byte, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	value, exists := s.StateData[key]
	return value, exists
}

// SetState sets a value in the shard's state
func (s *Shard) SetState(key string, value []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.StateData[key] = value
	s.TxCount++
	s.LastActivity = time.Now().Unix()
}

// DeleteState removes a key from the shard's state
func (s *Shard) DeleteState(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.StateData, key)
}

// AdaptiveShardMonitor monitors and manages shard splits/merges
func AdaptiveShardMonitor(intervalSec int, txThreshold int, mergeThreshold int) {
	go func() {
		for {
			time.Sleep(time.Duration(intervalSec) * time.Second)
			shardMutex.Lock()
			for _, shard := range ShardRegistry {
				if shard.TxCount > txThreshold {
					childID1 := shard.ID + "-auto1"
					childID2 := shard.ID + "-auto2"
					_ = SplitShard(shard.ID, childID1, childID2)
					shard.TxCount = 0
				}
				if shard.TxCount < mergeThreshold && len(shard.ChildrenIDs) == 2 {
					_ = MergeShards(shard.ID, shard.ChildrenIDs[0], shard.ChildrenIDs[1])
					shard.TxCount = 0
				}
			}
			shardMutex.Unlock()
		}
	}()
}

// TestShardCompactState tests the shard state compression and archival
func TestShardCompactState() {
	shard := CreateShard("test-shard", "")
	shard.SetState("key1", []byte("value1"))
	shard.SetState("key2", []byte("value2"))
	
	fmt.Println("Original shard state size:", len(shard.StateData))
	
	// Test state pruning
	pruned := make(map[string][]byte)
	shard.mutex.Lock()
	for k, v := range shard.StateData {
		if len(v) > 10 { // Example pruning condition
			pruned[k] = v
			delete(shard.StateData, k)
		}
	}
	shard.mutex.Unlock()
	
	fmt.Println("After pruning state size:", len(shard.StateData))
}

// GetShardByID retrieves a shard by its ID
func GetShardByID(id string) (*Shard, error) {
	shardMutex.RLock()
	defer shardMutex.RUnlock()

	shard, exists := ShardRegistry[id]
	if !exists {
		return nil, fmt.Errorf("shard %s not found", id)
	}
	return shard, nil
}

// SplitShard splits a shard into two child shards
func SplitShard(parentID, childID1, childID2 string) error {
	parent, err := GetShardByID(parentID)
	if err != nil {
		return err
	}

	// Create child shards
	child1 := CreateShard(childID1, parentID)
	child2 := CreateShard(childID2, parentID)

	// Split state between children (simplified example)
	parent.mutex.Lock()
	i := 0
	for k, v := range parent.StateData {
		if i%2 == 0 {
			child1.SetState(k, v)
		} else {
			child2.SetState(k, v)
		}
		i++
	}
	parent.mutex.Unlock()

	// Update parent's children IDs
	parent.mutex.Lock()
	parent.ChildrenIDs = []string{childID1, childID2}
	parent.mutex.Unlock()

	return nil
}

// MergeShards merges two child shards back into their parent
func MergeShards(parentID, childID1, childID2 string) error {
	parent, err := GetShardByID(parentID)
	if err != nil {
		return err
	}

	child1, err := GetShardByID(childID1)
	if err != nil {
		return err
	}

	child2, err := GetShardByID(childID2)
	if err != nil {
		return err
	}

	// Merge children's state into parent
	child1.mutex.RLock()
	for k, v := range child1.StateData {
		parent.SetState(k, v)
	}
	child1.mutex.RUnlock()

	child2.mutex.RLock()
	for k, v := range child2.StateData {
		parent.SetState(k, v)
	}
	child2.mutex.RUnlock()

	// Clear parent's children IDs
	parent.mutex.Lock()
	parent.ChildrenIDs = []string{}
	parent.mutex.Unlock()

	// Remove child shards from registry
	shardMutex.Lock()
	delete(ShardRegistry, childID1)
	delete(ShardRegistry, childID2)
	shardMutex.Unlock()

	return nil
}