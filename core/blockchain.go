package core

import (
	"errors"
	"log"
	"sync" // To protect access to the chain if we add concurrency later
	"bytes"
	
)

// Blockchain holds the sequence of blocks.
type Blockchain struct {
	mu     sync.RWMutex // Read-write mutex for thread safety
	blocks []*Block     // The chain of blocks
	// TODO: Add database backend later for persistence
	// TODO: Add index structures (e.g., map[hash]*Block) for faster lookups
}

// NewBlockchain creates a new blockchain with a genesis block.
func NewBlockchain(genesisTx *Transaction) (*Blockchain, error) {
	genesisBlock, err := NewGenesisBlock(genesisTx)
    if err != nil {
        return nil, err
    }
	log.Println("Genesis Block created.")
	return &Blockchain{blocks: []*Block{genesisBlock}}, nil
}

// AddBlock adds a new block to the blockchain after validation.
func (bc *Blockchain) AddBlock(transactions []*Transaction) (*Block, error) {
	bc.mu.Lock() // Lock for writing
	defer bc.mu.Unlock()

	prevBlock := bc.blocks[len(bc.blocks)-1]
	prevBlockHash, err := prevBlock.Hash()
	if err != nil {
		log.Printf("Error getting previous block hash: %v", err)
		return nil, err
	}

	newHeight := prevBlock.Header.Height + 1
	newBlock, err := NewBlock(transactions, newHeight, prevBlockHash)
    if err != nil {
        log.Printf("Error creating new block: %v", err)
        return nil, err
    }


	
	// 1. Check previous block hash pointer
	if !bytes.Equal(newBlock.Header.PrevBlockHash, prevBlockHash) {
		log.Printf("Validation Error: New block's PrevBlockHash (%x) does not match actual previous hash (%x)", newBlock.Header.PrevBlockHash, prevBlockHash)
		return nil, errors.New("invalid previous block hash")
	}

	// 2. Check block height
	if newBlock.Header.Height != newHeight {
		log.Printf("Validation Error: New block's height (%d) is incorrect, expected (%d)", newBlock.Header.Height, newHeight)
		return nil, errors.New("invalid block height")
	}

    // 3. Recalculate and verify block hash (integrity check)
    // Note: We trust CalculateHash for now, but a full node would re-validate everything
    calculatedHash, err := newBlock.CalculateHash()
    if err != nil {
        log.Printf("Validation Error: Failed to recalculate hash for new block: %v", err)
        return nil, err
    }
    blockHash, _ := newBlock.Hash() // Get cached/calculated hash
    if !bytes.Equal(calculatedHash, blockHash) {
         log.Printf("Validation Error: Block hash verification failed. Recalculated: %x, Stored: %x", calculatedHash, blockHash)
        return nil, errors.New("block hash verification failed")
    }


	// TODO: Add more validation: transaction validation, PoW check, signature checks etc.

	bc.blocks = append(bc.blocks, newBlock)
	log.Printf("Added Block %d with hash %x", newBlock.Header.Height, blockHash)
	return newBlock, nil
}

// GetBlockByHeight returns a block by its height.
func (bc *Blockchain) GetBlockByHeight(height uint32) (*Block, error) {
	bc.mu.RLock() // Lock for reading
	defer bc.mu.RUnlock()

	if int(height) >= len(bc.blocks) {
		return nil, errors.New("block height out of range")
	}
	return bc.blocks[height], nil
}

// GetLatestBlock returns the most recent block in the chain.
func (bc *Blockchain) GetLatestBlock() (*Block, error) {
    bc.mu.RLock()
    defer bc.mu.RUnlock()

    if len(bc.blocks) == 0 {
        return nil, errors.New("blockchain is empty")
    }
    return bc.blocks[len(bc.blocks)-1], nil
}

// GetChainHeight returns the height of the latest block.
func (bc *Blockchain) GetChainHeight() uint32 {
     bc.mu.RLock()
    defer bc.mu.RUnlock()
    if len(bc.blocks) == 0 {
        return 0 // Or handle appropriately, maybe return -1 or error?
    }
    // Height is index, so length 1 means height 0. Length N means max height N-1.
    return uint32(len(bc.blocks) - 1)
}

// GetBlocks returns a copy of all blocks (consider implications for large chains)
func (bc *Blockchain) GetBlocks() []*Block {
    bc.mu.RLock()
    defer bc.mu.RUnlock()
    // Return a copy to prevent external modification of the internal slice
    blocksCopy := make([]*Block, len(bc.blocks))
    copy(blocksCopy, bc.blocks)
    return blocksCopy
}