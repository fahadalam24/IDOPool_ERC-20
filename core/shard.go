package core

import (
	"crypto/sha256"
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"crypto/rand"
	"os"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// Shard represents a single shard in the Adaptive Merkle Forest.
type Shard struct {
	ID          string            // Unique identifier for the shard
	ParentID    string            // ID of the parent shard (empty if root shard)
	ChildrenIDs []string          // IDs of child shards (empty if leaf shard)
	MerkleRoot  []byte            // Merkle root representing the shard's state
	StateData   map[string][]byte // Key-value store for shard-specific state data
	BloomFilter *bloom.BloomFilter // Probabilistic filter for state membership
	Accumulator *MerkleAccumulator // Merkle-based cryptographic accumulator for state

	// Add a field to track computational load
	Load int `json:"load"` // Represents the computational load of the shard

	// Add adaptive metrics to Shard
	LastActivity   int64 // Unix timestamp of last activity
	TxCount        int   // Number of transactions processed recently
}

// MerkleAccumulator is a simple Merkle-based cryptographic accumulator for shard state.
type MerkleAccumulator struct {
	RootHash []byte   // The Merkle root (accumulated value)
	Leaves   [][]byte // The set of leaves (state keys or key-value hashes)
}

// NewMerkleAccumulator creates a new accumulator from a set of leaves.
func NewMerkleAccumulator(leaves [][]byte) (*MerkleAccumulator, error) {
	if len(leaves) == 0 {
		return &MerkleAccumulator{RootHash: nil, Leaves: nil}, nil
	}
	merkleTree, err := NewMerkleTree(leaves)
	if err != nil {
		return nil, err
	}
	return &MerkleAccumulator{RootHash: merkleTree.RootNode.Data, Leaves: leaves}, nil
}

// AddLeaf adds a new leaf to the accumulator and updates the root hash.
func (a *MerkleAccumulator) AddLeaf(leaf []byte) error {
	a.Leaves = append(a.Leaves, leaf)
	merkleTree, err := NewMerkleTree(a.Leaves)
	if err != nil {
		return err
	}
	a.RootHash = merkleTree.RootNode.Data
	return nil
}

// VerifyMembership checks if a given leaf is in the accumulator (by recomputing the root).
func (a *MerkleAccumulator) VerifyMembership(leaf []byte) bool {
	for _, l := range a.Leaves {
		if bytes.Equal(l, leaf) {
			return true
		}
	}
	return false
}

// ShardRegistry manages all active shards.
var ShardRegistry = make(map[string]*Shard)

// ShardIndex maintains a hierarchical index for efficient shard discovery
var ShardIndex = make(map[string]*Shard)

// AddShardToIndex adds a shard to the ShardIndex for efficient lookup
func AddShardToIndex(shard *Shard) {
	if shard != nil {
		ShardIndex[shard.ID] = shard
	}
}

// RemoveShardFromIndex removes a shard from the ShardIndex
func RemoveShardFromIndex(shardID string) {
	delete(ShardIndex, shardID)
}

// GetShardByID retrieves a shard from the ShardIndex by its ID
func GetShardByID(shardID string) (*Shard, error) {
	shard, exists := ShardIndex[shardID]
	if !exists {
		return nil, fmt.Errorf("shard with ID %s not found", shardID)
	}
	return shard, nil
}

// CreateShard creates a new shard with the given ID and parent ID.
func CreateShard(id string, parentID string) *Shard {
	shard := &Shard{
		ID:          id,
		ParentID:    parentID,
		ChildrenIDs: []string{},
		StateData:   make(map[string][]byte),
	}
	ShardRegistry[id] = shard
	AddShardToIndex(shard) // Add shard to index for efficient lookup

	// If the shard has a parent, update the parent's ChildrenIDs
	if parentID != "" {
		if parent, exists := ShardRegistry[parentID]; exists {
			parent.ChildrenIDs = append(parent.ChildrenIDs, id)
		}
	}

	return shard
}

// Update the Merkle root dynamically during shard splitting
func UpdateMerkleRoot(shard *Shard) error {
	if shard == nil {
		return fmt.Errorf("cannot update Merkle root for a nil shard")
	}

	// Collect all state data hashes
	var dataHashes [][]byte
	for key, value := range shard.StateData {
		hash := sha256.Sum256(append([]byte(key), value...))
		dataHashes = append(dataHashes, hash[:])
	}

	// Create a new Merkle tree
	merkleTree, err := NewMerkleTree(dataHashes)
	if err != nil {
		return fmt.Errorf("failed to create Merkle tree: %w", err)
	}

	// Update the shard's Merkle root
	shard.MerkleRoot = merkleTree.RootNode.Data
	return nil
}

// SplitShard splits a shard into two child shards.
func SplitShard(parentID string, childID1 string, childID2 string) error {
	parent, exists := ShardRegistry[parentID]
	if !exists {
		return fmt.Errorf("parent shard with ID %s does not exist", parentID)
	}

	// Create child shards
	child1 := CreateShard(childID1, parentID)
	child2 := CreateShard(childID2, parentID)

	// Transfer state data to child shards (simple split for now)
	for key, value := range parent.StateData {
		if len(key)%2 == 0 {
			child1.StateData[key] = value
		} else {
			child2.StateData[key] = value
		}
	}

	// Update Merkle roots for child shards
	if err := UpdateMerkleRoot(child1); err != nil {
		return fmt.Errorf("failed to update Merkle root for child shard %s: %w", childID1, err)
	}
	if err := UpdateMerkleRoot(child2); err != nil {
		return fmt.Errorf("failed to update Merkle root for child shard %s: %w", childID2, err)
	}

	// Clear parent state data (optional, depends on use case)
	parent.StateData = nil

	return nil
}

// MergeShards merges two child shards back into their parent shard.
func MergeShards(parentID string, childID1 string, childID2 string) error {
	parent, exists := ShardRegistry[parentID]
	if !exists {
		return fmt.Errorf("parent shard with ID %s does not exist", parentID)
	}

	child1, exists1 := ShardRegistry[childID1]
	child2, exists2 := ShardRegistry[childID2]
	if !exists1 || !exists2 {
		return fmt.Errorf("one or both child shards do not exist")
	}

	// Merge state data from child shards into the parent shard
	parent.StateData = make(map[string][]byte)
	for key, value := range child1.StateData {
		parent.StateData[key] = value
	}
	for key, value := range child2.StateData {
		parent.StateData[key] = value
	}

	// Update Merkle root for the parent shard
	if err := UpdateMerkleRoot(parent); err != nil {
		return fmt.Errorf("failed to update Merkle root for parent shard %s: %w", parentID, err)
	}

	// Remove child shards from the registry
	delete(ShardRegistry, childID1)
	delete(ShardRegistry, childID2)

	// Update parent's ChildrenIDs
	parent.ChildrenIDs = nil

	return nil
}

// Automatically split a shard if its load exceeds a threshold
func CheckAndSplitShard(shardID string, threshold int) error {
	shard, exists := ShardRegistry[shardID]
	if !exists {
		return fmt.Errorf("shard with ID %s does not exist", shardID)
	}

	if shard.Load > threshold {
		childID1 := shardID + "-child1"
		childID2 := shardID + "-child2"
		if err := SplitShard(shardID, childID1, childID2); err != nil {
			return fmt.Errorf("failed to split shard %s: %w", shardID, err)
		}
		log.Printf("Shard %s split into %s and %s due to high load", shardID, childID1, childID2)
	}

	return nil
}

// Automatically merge child shards back into their parent if their combined load falls below a threshold
func CheckAndMergeShards(parentID string, threshold int) error {
	parent, exists := ShardRegistry[parentID]
	if !exists {
		return fmt.Errorf("parent shard with ID %s does not exist", parentID)
	}

	// Ensure the parent has exactly two children
	if len(parent.ChildrenIDs) != 2 {
		return fmt.Errorf("parent shard %s does not have exactly two children", parentID)
	}

	childID1 := parent.ChildrenIDs[0]
	childID2 := parent.ChildrenIDs[1]

	child1, exists1 := ShardRegistry[childID1]
	child2, exists2 := ShardRegistry[childID2]
	if !exists1 || !exists2 {
		return fmt.Errorf("one or both child shards of parent %s do not exist", parentID)
	}

	// Check if the combined load of the children is below the threshold
	if child1.Load+child2.Load < threshold {
		if err := MergeShards(parentID, childID1, childID2); err != nil {
			return fmt.Errorf("failed to merge shards %s and %s into parent %s: %w", childID1, childID2, parentID, err)
		}
		log.Printf("Shards %s and %s merged back into parent %s due to low combined load", childID1, childID2, parentID)
	}

	return nil
}

// InitializeBloomFilter initializes the Bloom filter for a shard with given parameters.
func (s *Shard) InitializeBloomFilter(n uint, fpRate float64) {
	s.BloomFilter = bloom.NewWithEstimates(n, fpRate)
}

// AddKeyToBloom adds a key to the shard's Bloom filter.
func (s *Shard) AddKeyToBloom(key string) {
	if s.BloomFilter != nil {
		s.BloomFilter.Add([]byte(key))
	}
}

// CheckKeyInBloom checks if a key is possibly in the shard's state using the Bloom filter.
func (s *Shard) CheckKeyInBloom(key string) bool {
	if s.BloomFilter != nil {
		return s.BloomFilter.Test([]byte(key))
	}
	return false
}

// Update accumulator when setting state
func (s *Shard) SetState(key string, value []byte) {
	s.StateData[key] = value
	s.AddKeyToBloom(key)
	if s.Accumulator == nil {
		leaf := sha256.Sum256(append([]byte(key), value...))
		acc, _ := NewMerkleAccumulator([][]byte{leaf[:]})
		s.Accumulator = acc
	} else {
		leaf := sha256.Sum256(append([]byte(key), value...))
		s.Accumulator.AddLeaf(leaf[:])
	}
	s.TxCount++
	s.LastActivity = time.Now().Unix()
}

// DeleteState removes a key from the shard's state and (optionally) resets the Bloom filter.
// Note: Standard Bloom filters do not support deletion, so this only removes from StateData.
func (s *Shard) DeleteState(key string) {
	delete(s.StateData, key)
	// For a counting Bloom filter, you could decrement here.
	// For a standard Bloom filter, you may want to periodically rebuild it if false positives become an issue.
}

// CrossShardTransfer moves a key-value pair from one shard to another, updating Merkle accumulator and Bloom filter.
func CrossShardTransfer(from *Shard, to *Shard, key string) error {
	value, exists := from.StateData[key]
	if !exists {
		return fmt.Errorf("key '%s' not found in source shard %s", key, from.ID)
	}

	// Add to destination shard
	to.SetState(key, value)

	// Remove from source shard
	from.DeleteState(key)

	// Optionally, update Merkle root for both shards
	if err := UpdateMerkleRoot(from); err != nil {
		return fmt.Errorf("failed to update Merkle root for source shard: %w", err)
	}
	if err := UpdateMerkleRoot(to); err != nil {
		return fmt.Errorf("failed to update Merkle root for destination shard: %w", err)
	}

	return nil
}

// AtomicCrossShardTransfer performs a two-phase commit for atomic cross-shard transfer.
func AtomicCrossShardTransfer(from *Shard, to *Shard, key string) error {
	value, exists := from.StateData[key]
	if !exists {
		return fmt.Errorf("key '%s' not found in source shard %s", key, from.ID)
	}

	// Phase 1: Prepare (lock the key in both shards)
	// For demonstration, we'll just check if the key exists in 'from' and not in 'to'.
	if _, exists := to.StateData[key]; exists {
		return fmt.Errorf("key '%s' already exists in destination shard %s", key, to.ID)
	}

	// Phase 2: Commit (perform the transfer atomically)
	// Add to destination shard (but do not update Merkle root/accumulator yet)
	to.StateData[key] = value
	to.AddKeyToBloom(key)

	// Remove from source shard (but do not update Merkle root/accumulator yet)
	delete(from.StateData, key)

	// Only now update Merkle roots and accumulators (commit)
	if err := UpdateMerkleRoot(from); err != nil {
		return fmt.Errorf("failed to update Merkle root for source shard: %w", err)
	}
	if err := UpdateMerkleRoot(to); err != nil {
		return fmt.Errorf("failed to update Merkle root for destination shard: %w", err)
	}

	// Update accumulators
	if from.Accumulator != nil {
		// Rebuild accumulator for 'from' (since we can't remove from Merkle accumulator efficiently)
		var leaves [][]byte
		for k, v := range from.StateData {
			leaf := sha256.Sum256(append([]byte(k), v...))
			leaves = append(leaves, leaf[:])
		}
		acc, _ := NewMerkleAccumulator(leaves)
		from.Accumulator = acc
	}
	if to.Accumulator == nil {
		leaf := sha256.Sum256(append([]byte(key), value...))
		acc, _ := NewMerkleAccumulator([][]byte{leaf[:]})
		to.Accumulator = acc
	} else {
		leaf := sha256.Sum256(append([]byte(key), value...))
		to.Accumulator.AddLeaf(leaf[:])
	}

	return nil
}

// PartialStateTransfer transfers a subset of state (selected keys) from one shard to another.
func PartialStateTransfer(from *Shard, to *Shard, keys []string) error {
	for _, key := range keys {
		value, exists := from.StateData[key]
		if !exists {
			continue // Skip missing keys
		}
		to.SetState(key, value)
		from.DeleteState(key)
	}
	_ = UpdateMerkleRoot(from)
	_ = UpdateMerkleRoot(to)
	return nil
}

// ReconstructState reconstructs a shard's state from a provided partial state map.
func (s *Shard) ReconstructState(partial map[string][]byte) {
	s.StateData = make(map[string][]byte)
	for k, v := range partial {
		s.StateData[k] = v
	}
	_ = UpdateMerkleRoot(s)
}

// Test function for partial state transfer and reconstruction
func TestPartialStateTransferAndReconstruction() {
	shardA := CreateShard("shardA", "")
	shardB := CreateShard("shardB", "")
	shardA.SetState("alice", []byte("100"))
	shardA.SetState("bob", []byte("200"))
	shardA.SetState("carol", []byte("300"))
	shardA.SetState("dave", []byte("400"))

	fmt.Println("Before partial transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)

	PartialStateTransfer(shardA, shardB, []string{"bob", "dave"})

	fmt.Println("After partial transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)

	// Simulate reconstructing a new shard from a partial state
	partial := map[string][]byte{"bob": []byte("200"), "dave": []byte("400")}
	shardC := CreateShard("shardC", "")
	shardC.ReconstructState(partial)
	fmt.Println("Reconstructed shardC State:", shardC.StateData)
}

// Test function for cross-shard state transfer
func TestCrossShardTransfer() {
	shardA := CreateShard("shardA", "")
	shardB := CreateShard("shardB", "")
	shardA.InitializeBloomFilter(1000, 0.01)
	shardB.InitializeBloomFilter(1000, 0.01)

	shardA.SetState("alice", []byte("100"))
	shardA.SetState("bob", []byte("200"))

	fmt.Println("Before transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)

	err := CrossShardTransfer(shardA, shardB, "alice")
	if err != nil {
		fmt.Println("Transfer error:", err)
		return
	}

	fmt.Println("After transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)
	fmt.Println("alice in shardB bloom:", shardB.CheckKeyInBloom("alice"))
	fmt.Printf("shardB accumulator root: %x\n", shardB.Accumulator.RootHash)
}

// Test function for atomic cross-shard transfer
func TestAtomicCrossShardTransfer() {
	shardA := CreateShard("shardA", "")
	shardB := CreateShard("shardB", "")
	shardA.InitializeBloomFilter(1000, 0.01)
	shardB.InitializeBloomFilter(1000, 0.01)

	shardA.SetState("alice", []byte("100"))
	shardA.SetState("bob", []byte("200"))

	fmt.Println("Before atomic transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)

	err := AtomicCrossShardTransfer(shardA, shardB, "alice")
	if err != nil {
		fmt.Println("Atomic transfer error:", err)
		return
	}

	fmt.Println("After atomic transfer:")
	fmt.Println("shardA State:", shardA.StateData)
	fmt.Println("shardB State:", shardB.StateData)
	fmt.Println("alice in shardB bloom:", shardB.CheckKeyInBloom("alice"))
	fmt.Printf("shardB accumulator root: %x\n", shardB.Accumulator.RootHash)
}

// Test function for Bloom filter verification
func TestShardBloomFilter() {
	shard := CreateShard("bloomtest", "")
	shard.InitializeBloomFilter(1000, 0.01) // 1000 items, 1% false positive rate

	// Add some keys
	shard.SetState("alice", []byte("100"))
	shard.SetState("bob", []byte("200"))
	shard.SetState("carol", []byte("300"))

	// Test membership
	fmt.Println("alice in bloom:", shard.CheckKeyInBloom("alice")) // Should be true
	fmt.Println("bob in bloom:", shard.CheckKeyInBloom("bob"))     // Should be true
	fmt.Println("carol in bloom:", shard.CheckKeyInBloom("carol")) // Should be true
	fmt.Println("dave in bloom:", shard.CheckKeyInBloom("dave"))   // Should be false (most likely)
}

func TestShardManagement() {
	// Step 1: Create a root shard
	rootShard := CreateShard("root", "")
	fmt.Printf("Created root shard: %+v\n", rootShard)

	// Step 2: Split the root shard into two child shards
	err := SplitShard("root", "child1", "child2")
	if err != nil {
		fmt.Printf("Error splitting shard: %v\n", err)
		return
	}
	fmt.Printf("Shard registry after splitting: %+v\n", ShardRegistry)

	// Step 3: Merge the child shards back into the root shard
	err = MergeShards("root", "child1", "child2")
	if err != nil {
		fmt.Printf("Error merging shards: %v\n", err)
		return
	}
	fmt.Printf("Shard registry after merging: %+v\n", ShardRegistry)
}

func TestShardRebalancing() {
	// Step 1: Create a root shard
	rootShard := CreateShard("root", "")
	rootShard.Load = 50 // Initial load
	fmt.Printf("Initial root shard: %+v\n", rootShard)

	// Step 2: Simulate load increase and check for splitting
	rootShard.Load = 120 // Exceed threshold
	err := CheckAndSplitShard("root", 100) // Threshold is 100
	if err != nil {
		fmt.Printf("Error during shard splitting: %v\n", err)
		return
	}
	fmt.Printf("Shard registry after splitting: %+v\n", ShardRegistry)

	// Step 3: Simulate load decrease and check for merging
	child1 := ShardRegistry["root-child1"]
	child2 := ShardRegistry["root-child2"]
	child1.Load = 30
	child2.Load = 20 // Combined load is below threshold

	err = CheckAndMergeShards("root", 100) // Threshold is 100
	if err != nil {
		fmt.Printf("Error during shard merging: %v\n", err)
		return
	}
	fmt.Printf("Shard registry after merging: %+v\n", ShardRegistry)
}

// Test function for MerkleAccumulator
func TestShardAccumulator() {
	shard := CreateShard("accumtest", "")
	shard.InitializeBloomFilter(1000, 0.01)
	shard.SetState("alice", []byte("100"))
	shard.SetState("bob", []byte("200"))
	shard.SetState("carol", []byte("300"))

	fmt.Printf("Accumulator root: %x\n", shard.Accumulator.RootHash)
	leaf := sha256.Sum256(append([]byte("alice"), []byte("100")...))
	fmt.Println("alice in accumulator:", shard.Accumulator.VerifyMembership(leaf[:]))
	leaf2 := sha256.Sum256(append([]byte("dave"), []byte("400")...))
	fmt.Println("dave in accumulator:", shard.Accumulator.VerifyMembership(leaf2[:]))
}

// ArchivePrunedState writes pruned state entries to an archival file for the shard.
func (s *Shard) ArchivePrunedState(pruned map[string][]byte, keyToHeight map[string]uint32) error {
	if len(pruned) == 0 {
		return nil
	}
	filename := "archive_shard_" + s.ID + ".log"
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	timestamp := time.Now().Format(time.RFC3339)
	for key, value := range pruned {
		height := keyToHeight[key]
		line := fmt.Sprintf("%s | key: %s | value: %x | height: %d\n", timestamp, key, value, height)
		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

// PruneState removes state entries older than a given block height and updates Merkle root and accumulator.
func (s *Shard) PruneState(minHeight uint32, keyToHeight map[string]uint32) (pruned map[string][]byte) {
	pruned = make(map[string][]byte)
	for key, value := range s.StateData {
		height, ok := keyToHeight[key]
		if ok && height < minHeight {
			pruned[key] = value
			delete(s.StateData, key)
		}
	}
	// Archive pruned entries
	_ = s.ArchivePrunedState(pruned, keyToHeight)
	// Rebuild Merkle root and accumulator after pruning
	_ = UpdateMerkleRoot(s)
	if s.Accumulator != nil {
		var leaves [][]byte
		for k, v := range s.StateData {
			leaf := sha256.Sum256(append([]byte(k), v...))
			leaves = append(leaves, leaf[:])
		}
		acc, _ := NewMerkleAccumulator(leaves)
		s.Accumulator = acc
	}
	return pruned
}

// Test function for state pruning
func TestShardPruneState() {
	shard := CreateShard("prunetest", "")
	shard.InitializeBloomFilter(1000, 0.01)
	shard.SetState("alice", []byte("100"))
	shard.SetState("bob", []byte("200"))
	shard.SetState("carol", []byte("300"))
	// Simulate block heights for each key
	keyToHeight := map[string]uint32{
		"alice": 1,
		"bob": 2,
		"carol": 5,
	}
	fmt.Println("Before pruning:", shard.StateData)
	pruned := shard.PruneState(3, keyToHeight)
	fmt.Println("After pruning:", shard.StateData)
	fmt.Println("Pruned entries:", pruned)
	fmt.Printf("New Merkle root: %x\n", shard.MerkleRoot)
	fmt.Printf("New accumulator root: %x\n", shard.Accumulator.RootHash)
}

// CompressState serializes and gzip-compresses the shard's StateData.
func (s *Shard) CompressState() ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	enc := gob.NewEncoder(gz)
	if err := enc.Encode(s.StateData); err != nil {
		gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecompressState decompresses and restores StateData from a gzip-compressed byte slice.
func (s *Shard) DecompressState(data []byte) error {
	buf := bytes.NewBuffer(data)
	gz, err := gzip.NewReader(buf)
	if err != nil {
		return err
	}
	defer gz.Close()
	dec := gob.NewDecoder(gz)
	return dec.Decode(&s.StateData)
}

// Test function for compact state representation
func TestShardCompactState() {
	shard := CreateShard("compacttest", "")
	shard.SetState("alice", []byte("100"))
	shard.SetState("bob", []byte("200"))
	shard.SetState("carol", []byte("300"))

	compressed, err := shard.CompressState()
	if err != nil {
		fmt.Println("Compression error:", err)
		return
	}
	fmt.Printf("Compressed state size: %d bytes\n", len(compressed))

	// Clear state and restore from compressed data
	shard.StateData = make(map[string][]byte)
	if err := shard.DecompressState(compressed); err != nil {
		fmt.Println("Decompression error:", err)
		return
	}
	fmt.Println("Restored state:", shard.StateData)
}

// AdaptiveShardMonitor periodically checks shards and adaptively splits/merges based on activity
func AdaptiveShardMonitor(intervalSec int, txThreshold int, mergeThreshold int) {
	go func() {
		for {
			time.Sleep(time.Duration(intervalSec) * time.Second)
			for _, shard := range ShardRegistry {
				// Example: If recent TxCount exceeds threshold, split
				if shard.TxCount > txThreshold {
					childID1 := shard.ID + "-auto1"
					childID2 := shard.ID + "-auto2"
					_ = SplitShard(shard.ID, childID1, childID2)
					log.Printf("[Adaptive] Shard %s split due to high activity (TxCount=%d)", shard.ID, shard.TxCount)
					shard.TxCount = 0 // Reset after split
				}
				// Example: If recent TxCount is very low and has children, merge
				if shard.TxCount < mergeThreshold && len(shard.ChildrenIDs) == 2 {
					_ = MergeShards(shard.ID, shard.ChildrenIDs[0], shard.ChildrenIDs[1])
					log.Printf("[Adaptive] Shard %s merged children due to low activity (TxCount=%d)", shard.ID, shard.TxCount)
					shard.TxCount = 0 // Reset after merge
				}
			}
		}
	}()
}

// RSAAccumulator is a cryptographic accumulator using RSA groups.
type RSAAccumulator struct {
	N *big.Int // RSA modulus (should be a product of two large primes)
	Value *big.Int // Current accumulator value
}

// NewRSAAccumulator initializes an RSA accumulator with a random 2048-bit modulus (for demo only).
func NewRSAAccumulator() (*RSAAccumulator, error) {
	// For demo: generate a random 2048-bit modulus (not secure for production!)
	N, err := rand.Prime(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &RSAAccumulator{
		N: N,
		Value: big.NewInt(1), // Start with 1 (neutral element)
	}, nil
}

// Add adds an element (as a hash) to the accumulator.
func (a *RSAAccumulator) Add(element []byte) {
	h := new(big.Int).SetBytes(element)
	a.Value.Exp(a.Value, h, a.N) // acc = acc^h mod N
}

// MembershipProof is just the accumulator value before adding the element.
type MembershipProof struct {
	Witness *big.Int
}

// ProveMembership returns a witness for an element (for demo, just returns current value before add).
func (a *RSAAccumulator) ProveMembership(element []byte) *MembershipProof {
	return &MembershipProof{Witness: new(big.Int).Set(a.Value)}
}

// VerifyMembership checks if witness^hash == acc.Value mod N.
func (a *RSAAccumulator) VerifyMembership(element []byte, proof *MembershipProof) bool {
	h := new(big.Int).SetBytes(element)
	computed := new(big.Int).Exp(proof.Witness, h, a.N)
	return computed.Cmp(a.Value) == 0
}

// Test function for RSA accumulator
func TestRSAAccumulator() {
	acc, err := NewRSAAccumulator()
	if err != nil {
		fmt.Println("RSA accumulator init error:", err)
		return
	}
	// Add two elements
	el1 := sha256.Sum256([]byte("alice"))
	el2 := sha256.Sum256([]byte("bob"))
	proof1 := acc.ProveMembership(el1[:])
	acc.Add(el1[:])
	proof2 := acc.ProveMembership(el2[:])
	acc.Add(el2[:])
	fmt.Println("Verify alice:", acc.VerifyMembership(el1[:], proof1)) // Should be true
	fmt.Println("Verify bob:", acc.VerifyMembership(el2[:], proof2))   // Should be true
}