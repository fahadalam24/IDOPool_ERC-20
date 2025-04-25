package core

import (
	"github.com/dgraph-io/badger/v4"
	"google.golang.org/protobuf/proto"
	"go-blockchain/pb"
	"log"
)

// Blockchain represents the blockchain with a persistent database.
type Blockchain struct {
	db      *badger.DB
	tipHash []byte // Hash of the latest block
}

// NewBlockchain initializes a new blockchain with persistence.
func NewBlockchain(genesisTx *Transaction, dbPath string) (*Blockchain, error) {
	// Open the BadgerDB database
	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		return nil, err
	}

	bc := &Blockchain{db: db}

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
	return bc.db.Update(func(txn *badger.Txn) error {
		blockHash, err := block.Hash()
		if err != nil {
			return err
		}

		blockProto, err := block.ToProto()
		if err != nil {
			return err
		}

		blockBytes, err := proto.Marshal(blockProto)
		if err != nil {
			return err
		}

		// Store the block by its hash
		err = txn.Set(blockHash, blockBytes)
		if err != nil {
			return err
		}

		// Update the last hash
		err = txn.Set([]byte("lh"), blockHash)
		if err != nil {
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

	err = bc.storeBlock(newBlock)
	if err != nil {
		return nil, err
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