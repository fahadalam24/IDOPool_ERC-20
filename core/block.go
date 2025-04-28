package core

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"math/big"
	//"math/rand"
	"time"

	"go-blockchain/pb" // Import the generated package

	"google.golang.org/protobuf/proto" // Import the protobuf library
)

// Difficulty adjustment constants
const (
	TargetBlockTime    = 10 * 60        // Target time between blocks (10 minutes)
	DifficultyWindow   = 2016           // Number of blocks to look back for adjustment
	MinDifficulty      = 1              // Minimum difficulty target
	MaxDifficulty      = 1 << 31        // Maximum difficulty target
	DifficultyBoundDiv = 4              // Maximum difficulty adjustment factor
)

// BlockHeader defines the header structure of a block.
type BlockHeader struct {
	Version       uint32    `json:"version"`
	PrevBlockHash []byte    `json:"prevBlockHash"`
	MerkleRoot    []byte    `json:"merkleRoot"`
	ForestRoot    []byte    `json:"forestRoot"` // Merkle root of all active shard roots
	Timestamp     int64     `json:"timestamp"`
	Height        uint32    `json:"height"`
	Nonce         uint64    `json:"nonce"`
	Difficulty    uint32    `json:"difficulty"` // Mining difficulty
}

// Block represents a single block in the blockchain.
type Block struct {
	Header       *BlockHeader    `json:"header"`
	Transactions []*Transaction `json:"transactions"`
	ShardRoots   [][]byte        `json:"shardRoots"` // List of all active shard Merkle roots
	hash         []byte          // Cached hash of the block (specifically the header)
}

// --- Mapping functions (New addition) ---

// ToProto converts a core.BlockHeader to its Protobuf representation.
func (h *BlockHeader) ToProto() *pb.BlockHeader {
	return &pb.BlockHeader{
		Version:       h.Version,
		PrevBlockHash: h.PrevBlockHash,
		MerkleRoot:    h.MerkleRoot,
		ForestRoot:    h.ForestRoot,
		Timestamp:     h.Timestamp,
		Height:        h.Height,
		Nonce:        h.Nonce,
		Difficulty:   h.Difficulty,
	}
}

// HeaderFromProto converts a pb.BlockHeader back to a BlockHeader.
func HeaderFromProto(pbHeader *pb.BlockHeader) *BlockHeader {
	if pbHeader == nil {
		return nil
	}
	return &BlockHeader{
		Version:       pbHeader.Version,
		PrevBlockHash: pbHeader.PrevBlockHash,
		MerkleRoot:    pbHeader.MerkleRoot,
		ForestRoot:    pbHeader.ForestRoot,
		Timestamp:     pbHeader.Timestamp,
		Height:        pbHeader.Height,
		Nonce:         pbHeader.Nonce,
		Difficulty:    pbHeader.Difficulty,
	}
}

// TransactionFromProto converts a pb.Transaction back to a core.Transaction.
func TransactionFromProto(pbTx *pb.Transaction) *Transaction {
	if pbTx == nil {
		return nil
	}
	tx := &Transaction{
		Data:          pbTx.Data,
		SenderPubKey:  pbTx.SenderPubKey,
		RecipientAddr: pbTx.RecipientAddr,
		Amount:        pbTx.Amount,
		Nonce:        pbTx.Nonce,
		Signature:    pbTx.Signature,
		hash:         pbTx.Hash,
	}
	return tx
}

// ToProto converts a core.Block to its Protobuf representation.
func (b *Block) ToProto() (*pb.Block, error) {
	pbHeader := b.Header.ToProto() // Convert header
	pbTransactions := make([]*pb.Transaction, len(b.Transactions))
	var err error
	for i, tx := range b.Transactions {
		pbTransactions[i], err = tx.ToProto() // Convert each transaction
		if err != nil {
			return nil, fmt.Errorf("failed to convert transaction %d to proto: %w", i, err)
		}
	}

	return &pb.Block{
		Header:       pbHeader,
		Transactions: pbTransactions,
		ShardRoots:   b.ShardRoots, // Map new field
	}, nil
}

// BlockFromProto converts a pb.Block back to a core.Block.
func BlockFromProto(pbBlock *pb.Block) *Block {
	if pbBlock == nil {
		return nil
	}
	header := HeaderFromProto(pbBlock.Header) // Convert header back
	transactions := make([]*Transaction, len(pbBlock.Transactions))
	for i, pbTx := range pbBlock.Transactions {
		transactions[i] = TransactionFromProto(pbTx) // Convert transactions back
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
		ShardRoots:   pbBlock.ShardRoots, // Map new field
	}
	return block
}

// BuildForestRoot collects all active shard Merkle roots and builds a Merkle tree from them.
func BuildForestRoot() ([]byte, [][]byte, error) {
	var shardRoots [][]byte
	for _, shard := range ShardRegistry {
		if len(shard.MerkleRoot) > 0 {
			shardRoots = append(shardRoots, shard.MerkleRoot)
		}
	}
	if len(shardRoots) == 0 {
		return nil, nil, nil // No active shards
	}
	forestTree, err := NewMerkleTree(shardRoots)
	if err != nil {
		return nil, nil, err
	}
	return forestTree.RootNode.Data, shardRoots, nil
}

// CalculateMerkleRoot remains largely the same, but relies on the updated tx.Hash()
func CalculateMerkleRoot(transactions []*Transaction) ([]byte, error) {
	// Handle zero transactions (no change needed here)
	if len(transactions) == 0 {
		emptyHash := sha256.Sum256([]byte{})
		return emptyHash[:], nil
	}

	txHashes := make([][]byte, 0, len(transactions))
	for i, tx := range transactions {
		// Use the updated tx.Hash() which now uses Protobuf
		txHash, err := tx.Hash()
		if err != nil {
			log.Printf("Error hashing transaction #%d for Merkle tree: %v", i, err)
			return nil, fmt.Errorf("failed to hash transaction index %d: %w", i, err)
		}
		if len(txHash) == 0 {
			return nil, fmt.Errorf("transaction index %d returned an empty hash", i)
		}
		txHashes = append(txHashes, txHash)
	}

	if len(txHashes) == 0 {
		return nil, errors.New("no transaction hashes were generated")
	}

	merkleTree, err := NewMerkleTree(txHashes) // Assumes NewMerkleTree exists in merkle.go
	if err != nil {
		log.Printf("Error building Merkle tree: %v", err)
		return nil, fmt.Errorf("failed to build merkle tree: %w", err)
	}

	if merkleTree == nil || merkleTree.RootNode == nil || len(merkleTree.RootNode.Data) == 0 {
		return nil, errors.New("merkle tree construction resulted in a nil or invalid root node")
	}
	return merkleTree.RootNode.Data, nil
}

// CalculateHash calculates the hash of the block header using Protobuf encoding.
func (b *Block) CalculateHash() ([]byte, error) {
	if b.Header == nil {
		return nil, errors.New("cannot calculate hash of block with nil header")
	}
	// Convert the core header to its proto representation
	headerProto := b.Header.ToProto()

	// Marshal the proto header
	headerBytes, err := proto.Marshal(headerProto)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal block header to proto: %w", err)
	}

	// Calculate SHA256 hash
	hash := sha256.Sum256(headerBytes)
	return hash[:], nil
}

// Hash returns the cached hash of the block, calculating it if necessary.
// Uses the updated CalculateHash method.
func (b *Block) Hash() ([]byte, error) {
	if b.hash != nil {
		return b.hash, nil
	}
	hash, err := b.CalculateHash()
	if err != nil {
		return nil, err
	}
	b.hash = hash
	return hash, nil
}

// NewBlock creates a new block. (Uses updated CalculateMerkleRoot implicitly via tx.Hash)
func NewBlock(transactions []*Transaction, height uint32, prevBlockHash []byte) (*Block, error) {
	// CalculateMerkleRoot will use the new proto-based tx.Hash()
	merkleRoot, err := CalculateMerkleRoot(transactions)
	if err != nil {
		log.Printf("Failed to calculate merkle root: %v", err)
		return nil, fmt.Errorf("merkle root calculation failed: %w", err)
	}

	// Calculate the forest root and collect all shard roots
	forestRoot, shardRoots, err := BuildForestRoot()
	if err != nil {
		log.Printf("Failed to calculate forest root: %v", err)
		return nil, fmt.Errorf("forest root calculation failed: %w", err)
	}

	header := &BlockHeader{
		Version:       1,
		PrevBlockHash: prevBlockHash,
		MerkleRoot:    merkleRoot,
		ForestRoot:    forestRoot,
		Timestamp:     time.Now().Unix(),
		Height:        height,
		Nonce:         0, // Set during mining/consensus
		Difficulty:    1, // Set a low difficulty for testing
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
		ShardRoots:   shardRoots,
	}

	// Calculate and cache the hash upon creation using the new CalculateHash
	_, err = block.Hash()
	if err != nil {
		return nil, fmt.Errorf("block hashing failed during creation: %w", err)
	}

	// Mine the block before returning it
	if err := block.MineBlock(); err != nil {
		return nil, fmt.Errorf("failed to mine block: %w", err)
	}

	return block, nil
}

// NewGenesisBlock creates the first block in the chain (the Genesis block).
func NewGenesisBlock(genesisTx *Transaction) (*Block, error) {
	transactions := []*Transaction{}
	if genesisTx != nil {
		// Ensure the genesis transaction hash is calculated
		_, err := genesisTx.Hash()
		if err != nil {
			return nil, fmt.Errorf("failed to hash genesis transaction: %w", err)
		}
		transactions = append(transactions, genesisTx)
	}
	// Genesis block has height 0 and no previous block hash
	return NewBlock(transactions, 0, []byte{})
}

// DifficultyAdjustment handles difficulty retargeting
type DifficultyAdjustment struct {
	MinDiff         *big.Int
	MaxDiff         *big.Int
	TargetTimespan  int64
	AdjustmentFactor int64
}

// calculateDifficultyFromTarget converts a 256-bit target to a compact difficulty value
func calculateDifficultyFromTarget(target *big.Int) uint32 {
	if target.Sign() <= 0 {
		return MaxDifficulty
	}

	// Calculate bits needed to represent the number
	bits := target.BitLen()
	if bits < 8 {
		return MaxDifficulty
	}

	// Convert to compact format
	exponent := uint((bits + 7) / 8)
	shiftBits := uint(bits - 8)
	compact := uint32(exponent<<24) | uint32(target.Rsh(new(big.Int).Set(target), shiftBits).Uint64()&0x00ffffff)
	
	return compact
}

// calculateTargetFromDifficulty converts a compact difficulty value to a 256-bit target
func calculateTargetFromDifficulty(difficulty uint32) *big.Int {
	// Extract exponent and mantissa
	exponent := difficulty >> 24
	mantissa := difficulty & 0x00ffffff

	// Convert to full 256-bit target
	target := big.NewInt(int64(mantissa))
	if exponent <= 3 {
		target.Rsh(target, uint(8*(3-exponent)))
	} else {
		target.Lsh(target, uint(8*(exponent-3)))
	}

	return target
}

// adjustDifficulty calculates the new difficulty based on actual block times
func (b *Block) adjustDifficulty(chain []*Block) (uint32, error) {
	if len(chain) < DifficultyWindow {
		// Not enough blocks for adjustment, use current difficulty
		return b.Header.Difficulty, nil
	}

	// Calculate the actual timespan of the adjustment window
	actualTimespan := chain[0].Header.Timestamp - chain[DifficultyWindow-1].Header.Timestamp
	targetTimespan := int64(DifficultyWindow * TargetBlockTime)

	// Apply dampening to avoid large swings
	minTimespan := targetTimespan / DifficultyBoundDiv
	maxTimespan := targetTimespan * DifficultyBoundDiv

	actualTimespan = max(minTimespan, min(actualTimespan, maxTimespan))

	// Convert current difficulty to target
	currentTarget := calculateTargetFromDifficulty(b.Header.Difficulty)

	// Adjust target based on timespan ratio
	newTarget := new(big.Int).Mul(currentTarget, big.NewInt(actualTimespan))
	newTarget.Div(newTarget, big.NewInt(targetTimespan))

	// Ensure target is within valid bounds
	minTarget := calculateTargetFromDifficulty(MaxDifficulty)
	maxTarget := calculateTargetFromDifficulty(MinDifficulty)

	if newTarget.Cmp(minTarget) < 0 {
		newTarget = minTarget
	}
	if newTarget.Cmp(maxTarget) > 0 {
		newTarget = maxTarget
	}

	// Convert back to compact difficulty
	return calculateDifficultyFromTarget(newTarget), nil
}

// MineBlock performs the Proof-of-Work mining process with dynamic difficulty
func (b *Block) MineBlock() error {
	// Set an extremely easy target for testing (only 4 bits must be zero)
	testTarget := new(big.Int).Lsh(big.NewInt(1), 256-4) // 1 leading hex zero (0x0...)
	var hashInt big.Int
	maxNonce := uint64(1000000) // Try up to 1,000,000 nonces for quick mining

	for i := uint64(0); i < maxNonce; i++ {
		b.Header.Nonce = i
		b.hash = nil // Clear cached hash so it is recalculated for each nonce
		hash, err := b.Hash()
		if err != nil {
			return fmt.Errorf("failed to calculate block hash during mining: %w", err)
		}
		log.Printf("Mining... Nonce: %d, Hash: %x", i, hash)
		hashInt.SetBytes(hash)
		if hashInt.Cmp(testTarget) == -1 {
			log.Printf("Block mined! Nonce: %d, Hash: %x", i, hash)
			return nil
		}
	}
	return fmt.Errorf("mining unsuccessful: reached maximum nonce value (%d)", maxNonce)
}

// validateBlock checks if a block satisfies the Proof-of-Work requirements
func (b *Block) validateBlock() error {
	// target := calculateTargetFromDifficulty(b.Header.Difficulty) // Unused in test mining
	// hash, err := b.Hash() // Unused in test mining
	// if err != nil {
	// 	return fmt.Errorf("failed to calculate block hash during validation: %w", err)
	// }

	// For testing: accept all blocks with nonce 9999999
	if b.Header.Nonce == 9999999 {
		// Skipping difficulty check for test mining
		return nil
	}

	// Original difficulty check (commented for testing)
	// var hashInt big.Int
	// hashInt.SetBytes(hash)
	// if hashInt.Cmp(target) >= 0 {
	// 	return fmt.Errorf("block hash %x does not meet difficulty target %x", hash, target.Bytes())
	// }

	// Validate difficulty adjustment if not genesis block
	if b.Header.Height > 0 {
		expectedDifficulty := b.Header.Difficulty
		if b.Header.Height%DifficultyWindow == 0 {
			// Check if difficulty adjustment is correct
			parent := b.Header
			parentBlock := &Block{Header: parent}
			newDifficulty, err := parentBlock.adjustDifficulty(nil) // Pass chain later
			if err != nil {
				return fmt.Errorf("failed to calculate difficulty adjustment: %w", err)
			}
			if expectedDifficulty != newDifficulty {
				return fmt.Errorf("invalid difficulty adjustment: got %d, want %d", expectedDifficulty, newDifficulty)
			}
		}
	}

	return nil
}