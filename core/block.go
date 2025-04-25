package core

import (
	//"bytes"
	"crypto/sha256"
	// "encoding/gob" // No longer needed for hashing
	"errors"
	"fmt"
	"log"
	"time"

	"go-blockchain/pb" // Import the generated package

	"google.golang.org/protobuf/proto" // Import the protobuf library
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
		ForestRoot:    h.ForestRoot, // Map new field
		Timestamp:     h.Timestamp,
		Height:        h.Height,
		Nonce:         h.Nonce,
	}
}

func HeaderFromProto(pbHeader *pb.BlockHeader) *BlockHeader {
	if pbHeader == nil {
		return nil
	}
	return &BlockHeader{
		Version:       pbHeader.Version,
		PrevBlockHash: pbHeader.PrevBlockHash,
		MerkleRoot:    pbHeader.MerkleRoot,
		ForestRoot:    pbHeader.ForestRoot, // Map new field
		Timestamp:     pbHeader.Timestamp,
		Height:        pbHeader.Height,
		Nonce:         pbHeader.Nonce,
	}
}

// ToProto converts a core.Transaction to its Protobuf representation.
func (t *Transaction) ToProto() (*pb.Transaction, error) {
	// Ensure hash is calculated before including it in the proto message
	// Note: We included 'hash' field in proto.Transaction
	// If hash depends on other fields, calculate it first.
	hash, err := t.Hash() // Calculate/get hash
	if err != nil {
		return nil, fmt.Errorf("failed to get tx hash for proto conversion: %w", err)
	}

	return &pb.Transaction{
		Data: t.Data,
		Hash: hash, // Include the calculated hash
		// Map other fields here if added later
	}, nil
}

// TransactionFromProto converts a pb.Transaction back to a core.Transaction.
func TransactionFromProto(pbTx *pb.Transaction) *Transaction {
	if pbTx == nil {
		return nil
	}
	tx := &Transaction{
		Data: pbTx.Data,
		hash: pbTx.Hash, // Store the hash from the proto message
		// Map other fields here if added later
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

// MineBlock performs the Proof-of-Work mining process for a block.
func (b *Block) MineBlock() error {
	log.Printf("Testing simulated nonce: %d", b.Header.Nonce)
	target := uint32(1 << (32 - b.Header.Difficulty))
	for i := uint64(1); i < 100000; i++ { // Test a larger range of nonce values
		b.Header.Nonce = i
		hash, err := b.Hash()
		if err != nil {
			return fmt.Errorf("failed to calculate block hash during mining simulation: %w", err)
		}

		// Convert the first 4 bytes of the hash to an integer
		var hashInt uint32
		for j := 0; j < 4; j++ {
			hashInt = (hashInt << 8) | uint32(hash[j])
		}

		if hashInt < target {
			log.Printf("Mining success with nonce %d", b.Header.Nonce)
			return nil
		}
	}

	return fmt.Errorf("no valid nonce found in the range")
}

// validateBlock checks if a block satisfies the Proof-of-Work requirements.
func (b *Block) validateBlock() error {
	target := uint32(1 << (32 - b.Header.Difficulty))
	hash, err := b.Hash()
	if err != nil {
		return fmt.Errorf("failed to calculate block hash during validation: %w", err)
	}

	// Convert the first 4 bytes of the hash to an integer
	var hashInt uint32
	for i := 0; i < 4; i++ {
		hashInt = (hashInt << 8) | uint32(hash[i])
	}

	if hashInt >= target {
		return fmt.Errorf("block does not satisfy Proof-of-Work requirements")
	}

	return nil
}