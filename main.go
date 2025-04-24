package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"go-blockchain/core" // Use the correct module path
)

func main() {
	log.Println("Starting blockchain...")

	// Create a simple genesis transaction
	genesisTx := core.NewTransaction([]byte("Genesis Transaction - Network Start"))

	// Initialize the blockchain
	bc, err := core.NewBlockchain(genesisTx)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}

	log.Println("Blockchain initialized.")

	// Add a few blocks
	log.Println("Adding Block 1...")
	tx1 := core.NewTransaction([]byte("Alice sends 10 to Bob")) // This is correct, NewTransaction is in core
	tx2 := core.NewTransaction([]byte("Charlie mines reward")) // This is correct

	// FIX: Use core.Transaction for the slice type
	_, err = bc.AddBlock([]*core.Transaction{tx1, tx2})
	if err != nil { // Make sure you have the if statement wrapping the log
		log.Printf("Failed to add block 1: %v", err)
	}

	log.Println("Adding Block 2...")
	tx3 := core.NewTransaction([]byte("Bob sends 5 to Dave"))

	// FIX: Use core.Transaction for the slice type here too
	_, err = bc.AddBlock([]*core.Transaction{tx3})
	if err != nil { // Make sure you have the if statement wrapping the log
		log.Printf("Failed to add block 2: %v", err)
	}

	// Print information about the blocks
	log.Println("\n--- Blockchain Blocks ---")
	for _, block := range bc.GetBlocks() {
        blockHash, _ := block.Hash() // Error already handled on creation/add
		fmt.Printf("== Block %d ==\n", block.Header.Height)
		fmt.Printf("  Timestamp:     %d\n", block.Header.Timestamp)
		fmt.Printf("  PrevBlockHash: %s\n", hex.EncodeToString(block.Header.PrevBlockHash))
		fmt.Printf("  MerkleRoot:    %s\n", hex.EncodeToString(block.Header.MerkleRoot))
		fmt.Printf("  Nonce:         %d\n", block.Header.Nonce)
		fmt.Printf("  Hash:          %s\n", hex.EncodeToString(blockHash))
		fmt.Printf("  Transactions (%d):\n", len(block.Transactions))
		for i, tx := range block.Transactions {
            // tx here is already of type *core.Transaction because that's what's stored in the block
            txHash, _ := tx.Hash()
			fmt.Printf("    Tx %d: Data: %s | Hash: %s\n", i, string(tx.Data), hex.EncodeToString(txHash))
		}
		fmt.Println()
	}

	latestBlock, _ := bc.GetLatestBlock()
	latestBlockHash, _ := latestBlock.Hash()
	fmt.Printf("Current chain height: %d\n", bc.GetChainHeight())
	fmt.Printf("Latest Block Hash: %s\n", hex.EncodeToString(latestBlockHash))
}