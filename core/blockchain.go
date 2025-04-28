package core

import (
	"context"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/protobuf/proto"
	"go-blockchain/pb"
	"log"
)

// Blockchain represents the blockchain with a persistent database.
type Blockchain struct {
	db        *badger.DB
	tipHash   []byte // Hash of the latest block
	consensus *ConsensusEngine // Consensus engine for block finalization
}

// NewBlockchain initializes a new blockchain with persistence.
func NewBlockchain(genesisTx *Transaction, dbPath string, consensus *ConsensusEngine) (*Blockchain, error) {
	// Open the BadgerDB database
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		return nil, err
	}

	bc := &Blockchain{db: db, consensus: consensus}

	// Check if the database is empty
	err = db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte("lh")) // "lh" stands for "last hash"
		if err == badger.ErrKeyNotFound {
			// Database is empty, create the genesis block
			log.Println("Creating genesis block...")
			genesisBlock, err := NewGenesisBlock(genesisTx)
			if err != nil {
				return err
			}
			return bc.storeBlock(genesisBlock)
		}
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	// Load the tip hash
	err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("lh"))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			bc.tipHash = append([]byte{}, val...)
			return nil
		})
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	return bc, nil
}

// storeBlock saves a block to the database.
func (bc *Blockchain) storeBlock(block *Block) error {
	if bc == nil {
		return fmt.Errorf("storeBlock: Blockchain instance is nil")
	}
	if bc.db == nil {
		return fmt.Errorf("storeBlock: database is nil")
	}
	if block == nil {
		return fmt.Errorf("storeBlock: block is nil")
	}
	if block.Header == nil {
		return fmt.Errorf("storeBlock: block.Header is nil")
	}
	blockHash, err := block.Hash()
	if err != nil {
		log.Printf("storeBlock: failed to calculate block hash: %v", err)
		return err
	}

	blockProto, err := block.ToProto()
	if err != nil {
		log.Printf("storeBlock: failed to convert block to proto: %v", err)
		return err
	}

	blockBytes, err := proto.Marshal(blockProto)
	if err != nil {
		log.Printf("storeBlock: failed to marshal block proto: %v", err)
		return err
	}

	return bc.db.Update(func(txn *badger.Txn) error {
		if blockHash == nil {
			log.Println("storeBlock: blockHash is nil inside transaction")
			return fmt.Errorf("blockHash is nil")
		}
		if blockBytes == nil {
			log.Println("storeBlock: blockBytes is nil inside transaction")
			return fmt.Errorf("blockBytes is nil")
		}
		// Store the block by its hash
		err = txn.Set(blockHash, blockBytes)
		if err != nil {
			log.Printf("storeBlock: failed to set block in DB: %v", err)
			return err
		}
		// Update the last hash
		err = txn.Set([]byte("lh"), blockHash)
		if err != nil {
			log.Printf("storeBlock: failed to set last hash in DB: %v", err)
			return err
		}
		bc.tipHash = blockHash
		return nil
	})
}

// getBlockByHash retrieves a block from the database by its hash.
func (bc *Blockchain) getBlockByHash(hash []byte) (*Block, error) {
	var block *Block
	err := bc.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(hash)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			blockProto := &pb.Block{}
			if err := proto.Unmarshal(val, blockProto); err != nil {
				return err
			}
			block = BlockFromProto(blockProto)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return block, nil
}

// AddBlock adds a new block to the blockchain.
func (bc *Blockchain) AddBlock(transactions []*Transaction) (*Block, error) {
	latestBlock, err := bc.getBlockByHash(bc.tipHash)
	if err != nil {
		return nil, err
	}

	newBlock, err := NewBlock(transactions, latestBlock.Header.Height+1, bc.tipHash)
	if err != nil {
		return nil, err
	}

	// Validate the block before adding it to the blockchain
	if err := newBlock.validateBlock(); err != nil {
		return nil, fmt.Errorf("block validation failed: %w", err)
	}

	// Require consensus before storing the block
	if bc.consensus != nil {
		ctx := context.Background()
		if err := bc.consensus.StartConsensus(ctx, newBlock); err != nil {
			return nil, fmt.Errorf("consensus failed: %w", err)
		}
	} else {
		// Fallback: store block directly if no consensus engine (should not happen in production)
		if err = bc.storeBlock(newBlock); err != nil {
			return nil, err
		}
	}

	return newBlock, nil
}

// Close closes the database.
func (bc *Blockchain) Close() {
	bc.db.Close()
}

// GetChainHeight returns the current height of the blockchain.
func (bc *Blockchain) GetChainHeight() (uint32, error) {
	latestBlock, err := bc.getBlockByHash(bc.tipHash)
	if err != nil {
		return 0, err
	}
	return latestBlock.Header.Height, nil
}

// GetLatestBlock retrieves the latest block in the blockchain.
func (bc *Blockchain) GetLatestBlock() (*Block, error) {
	return bc.getBlockByHash(bc.tipHash)
}