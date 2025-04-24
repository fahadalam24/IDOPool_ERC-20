package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob" // Using Gob for block hashing - more efficient than JSON for binary data
	"log"
	"time"
	"errors"
	"fmt"
)

// BlockHeader defines the header structure of a block.
type BlockHeader struct {
	Version       uint32    `json:"version"`       // Block version number
	PrevBlockHash []byte    `json:"prevBlockHash"` // Hash of the previous block
	MerkleRoot    []byte    `json:"merkleRoot"`    // Root hash of the transaction Merkle tree
	Timestamp     int64     `json:"timestamp"`     // Unix timestamp
	Height        uint32    `json:"height"`        // Block height (index in the chain)
	Nonce         uint64    `json:"nonce"`         // Nonce used for Proof-of-Work (or other consensus)
	// TODO: Add difficulty target later
}

// Block represents a single block in the blockchain.
type Block struct {
	Header       *BlockHeader    `json:"header"`       // Pointer to the block header
	Transactions []*Transaction `json:"transactions"` // List of transactions included in the block
	hash         []byte          // Cached hash of the block (specifically the header)
}

// CalculateMerkleRoot calculates a simple Merkle root for the block's transactions.
func CalculateMerkleRoot(transactions []*Transaction) ([]byte, error) {
	// --- Case: Zero Transactions ---
	if len(transactions) == 0 {
		// Use the predefined empty hash constant from merkle.go
		// Need to access it if defined there, or redefine here.
		// For simplicity, let's assume it's accessible or just recalculate here for now.
		// emptyHash := sha256.Sum256([]byte{}) // Option 1: Recalculate
		// return emptyHash[:], nil
		// Option 2: Assume accessible via core.emptyMerkleRootHash (requires export or different structure)
		// Let's stick to recalculating here for simplicity unless we restructure package access.
		emptyHash := sha256.Sum256([]byte{}) // Using recalculation for now
		return emptyHash[:], nil
		// To use the constant directly, you'd need:
		// 1. Make emptyMerkleRootHash exported (e.g., EmptyMerkleRootHash) in merkle.go
		// 2. Access it as core.EmptyMerkleRootHash[:] here.
	}

	// --- Step 1: Extract Transaction Hashes ---
	txHashes := make([][]byte, 0, len(transactions)) // Pre-allocate slice capacity
	for i, tx := range transactions {
		txHash, err := tx.Hash() // Assumes tx.Hash() returns the SHA256 hash []byte
		if err != nil {
			log.Printf("Error hashing transaction #%d for Merkle tree: %v", i, err)
			// Return error immediately to prevent block creation with potentially invalid/missing tx data.
			// Consider how critical a single tx hash failure is - should it halt block creation? Usually yes.
			return nil, fmt.Errorf("failed to hash transaction index %d: %w", i, err)
		}
		if len(txHash) == 0 {
			// Additional check: Ensure the hash function didn't return an empty slice unexpectedly
			return nil, fmt.Errorf("transaction index %d returned an empty hash", i)
		}
		txHashes = append(txHashes, txHash)
	}

	// Defensive check (though unlikely if tx loop returns error on failure)
	if len(txHashes) == 0 /* && len(transactions) > 0 */ {
		// This implies all transactions failed to hash, and the errors were somehow missed.
		return nil, errors.New("no transaction hashes were generated despite non-empty transaction list")
	}

	// --- Step 2: Build the Merkle Tree ---
	// NewMerkleTree itself handles the cases of 1, even, or odd number of hashes.
	merkleTree, err := NewMerkleTree(txHashes)
	if err != nil {
		log.Printf("Error building Merkle tree: %v", err)
		return nil, fmt.Errorf("failed to build merkle tree: %w", err)
	}

	// --- Step 3: Return the Root Hash ---
	if merkleTree == nil || merkleTree.RootNode == nil || len(merkleTree.RootNode.Data) == 0 {
		// Final sanity check on the result from NewMerkleTree
		return nil, errors.New("merkle tree construction resulted in a nil or invalid root node")
	}
	return merkleTree.RootNode.Data, nil
}

// CalculateHash calculates the hash of the block header using Gob encoding.
func (b *Block) CalculateHash() ([]byte, error) {
	var buf bytes.Buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(b.Header) // Only encode the header for the block hash
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(buf.Bytes())
	return hash[:], nil
}

// Hash returns the cached hash of the block, calculating it if necessary.
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

// NewBlock creates a new block. (Ensure CalculateMerkleRoot error is handled)
func NewBlock(transactions []*Transaction, height uint32, prevBlockHash []byte) (*Block, error) {
	merkleRoot, err := CalculateMerkleRoot(transactions)
	if err != nil {
		// Propagate the error from CalculateMerkleRoot
		log.Printf("Failed to calculate merkle root: %v", err)
		return nil, fmt.Errorf("merkle root calculation failed: %w", err) // Return the error
	}

	header := &BlockHeader{
		Version:       1,
		PrevBlockHash: prevBlockHash,
		MerkleRoot:    merkleRoot, // Use the calculated root
		Timestamp:     time.Now().Unix(),
		Height:        height,
		Nonce:         0,
	}

	block := &Block{
		Header:       header,
		Transactions: transactions,
	}

	// Calculate and cache the hash upon creation
	_, err = block.Hash() // Hash() itself calls CalculateHash() which uses the header
	if err != nil {
		return nil, fmt.Errorf("block hashing failed: %w", err) // Return error if hashing fails
	}

	return block, nil
}

// NewGenesisBlock creates the first block in the chain (the Genesis block).
func NewGenesisBlock(genesisTx *Transaction) (*Block, error) {
	transactions := []*Transaction{}
    if genesisTx != nil {
        transactions = append(transactions, genesisTx)
    }
	// Genesis block has height 0 and no previous block hash
	return NewBlock(transactions, 0, []byte{})
}