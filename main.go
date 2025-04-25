package main

import (
	"context"
	"encoding/hex"
	"flag" // Import flag package
	"fmt"
	"log"
	"os" // For signal handling
	"os/signal"
	"strings" // For splitting bootstrap peers
	"syscall"
	"time"

	"go-blockchain/core"
	"go-blockchain/network" // Import the new network package
)

func main() {
	// --- Command Line Flags ---
	listenPort := flag.Int("port", 0, "Port number for the node to listen on (0 for random)")
	bootstrapPeersStr := flag.String("peers", "", "Comma-separated list of bootstrap peer multiaddresses")
	useMDNS := flag.Bool("mdns", false, "Enable mDNS discovery for local network") // Default to false
	mdnsTag := flag.String("mdnsTag", "go-blockchain-dev", "mDNS service tag for discovery")
	flag.Parse()

	log.Println("Starting blockchain node...")

	// --- Initialize Core Blockchain ---
	// (Keep genesis simple for now, can enhance later)
	genesisTx := core.NewTransaction([]byte("Genesis Transaction"))
	// Initialize the blockchain with persistence
	dbPath := fmt.Sprintf("blockchain_data_%d", *listenPort)
	bc, err := core.NewBlockchain(genesisTx, dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain: %v", err)
	}
	defer bc.Close() // Ensure the database is closed on exit

	log.Println("Core blockchain initialized.")

	// --- Initialize Networking ---
	ctx, cancel := context.WithCancel(context.Background()) // Create a cancellable context
	defer cancel()                                         // Ensure cancel is called on exit

	node, err := network.NewNetworkNode(ctx, *listenPort)
	if err != nil {
		log.Fatalf("Failed to initialize network node: %v", err)
	}
	defer node.Close() // Ensure node is closed cleanly

	// Start mDNS Discovery if enabled
	if *useMDNS {
		err = node.StartDiscovery(ctx, *mdnsTag)
		if err != nil {
			log.Printf("Warning: Failed to start mDNS discovery: %v", err)
			// Continue execution even if mDNS fails
		} else {
			log.Println("mDNS Discovery started.")
		}
	} else {
		log.Println("mDNS Discovery disabled.")
	}

	// Parse and connect to Bootstrap Peers
	var bootstrapPeers []string
	if *bootstrapPeersStr != "" {
		bootstrapPeers = strings.Split(*bootstrapPeersStr, ",")
	}
	// Run connection attempt in a separate goroutine to avoid blocking main startup
	go node.ConnectToBootstrapPeers(ctx, bootstrapPeers)

	// --- Print Node's Listening Address ---
	// Give libp2p a moment to potentially figure out external addresses if NAT traversal is enabled later
	// time.Sleep(1 * time.Second) // Optional delay
	nodeAddr, err := node.GetFullAddr()
	if err != nil {
		log.Printf("Could not get node's full address: %v", err)
	} else {
		fmt.Println("==================================================")
		fmt.Printf(" Node is running! Your Multiaddress is:\n %s\n", nodeAddr)
		fmt.Println("==================================================")
		fmt.Println("Pass this address to other nodes using the -peers flag.")

		// Save the address to a file
		err = os.WriteFile("node_address.txt", []byte(nodeAddr), 0644)
		if err != nil {
			log.Printf("Failed to save node address to file: %v", err)
		} else {
			log.Println("Node address saved to 'node_address.txt'.")
		}
	}

	// --- Keep the application running & handle shutdown ---
	log.Println("Node setup complete. Running...")
	// (Example: Broadcast a new block when created)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		blockCounter := 1
		for {
			select {
			case <-ticker.C:
				log.Printf("Creating and broadcasting block %d...", blockCounter)
				txData := fmt.Sprintf("Block %d Data - Time %d", blockCounter, time.Now().Unix())
				newTx := core.NewTransaction([]byte(txData))
				newBlock, err := bc.AddBlock([]*core.Transaction{newTx})
				if err != nil {
					log.Printf("Error adding block: %v", err)
					continue
				}

				blockProto, err := newBlock.ToProto()
				if err != nil {
					log.Printf("Error converting block to Protobuf: %v", err)
					continue
				}

				node.BroadcastBlock(ctx, blockProto)
				log.Printf("Broadcasted block %d", blockCounter)
				blockCounter++
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle received blocks in the network node
	node.RegisterStreamHandler()

	// Start adaptive shard monitor: check every 10 seconds, split if TxCount > 5, merge if TxCount < 2
	core.AdaptiveShardMonitor(10, 5, 2)

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh // Block until a signal is received

	log.Println("Shutdown signal received. Shutting down...")
	// Context cancellation (done by defer cancel()) will signal goroutines to stop.
	// Node closing is handled by defer node.Close().
	log.Println("Node shut down gracefully.")

	// --- Print blockchain info (optional, mainly for debugging now) ---
	log.Println("\n--- Final Blockchain State ---")
	printBlockchainInfo(bc) // Extracted printing logic into a function

	// Test shard rebalancing
	core.TestShardCompactState()
	core.TestMerkleProofOfShardInForest()
	os.Exit(0) // Exit after running the Merkle proof-of-inclusion test to show output only for the test
}

// printBlockchainInfo prints summary information about the blockchain.
func printBlockchainInfo(bc *core.Blockchain) {
	chainHeight, err := bc.GetChainHeight()
	if err != nil {
		log.Printf("Error retrieving chain height: %v", err)
	} else {
		fmt.Printf("Current chain height: %d\n", chainHeight)
	}
	latestBlock, err := bc.GetLatestBlock()
	if err == nil {
		latestBlockHash, _ := latestBlock.Hash()
		fmt.Printf("Latest Block Hash: %s\n", hex.EncodeToString(latestBlockHash))
	} else {
		fmt.Println("Blockchain is empty or error retrieving latest block.")
	}

	/* // Optionally print all blocks (can be verbose)
	   fmt.Println("\n--- All Blocks ---")
	   for _, block := range bc.GetBlocks() {
	       blockHash, _ := block.Hash() // Error handled on creation/add
	       fmt.Printf("== Block %d | Hash: %s ==\n", block.Header.Height, hex.EncodeToString(blockHash))
	       // Add more details if needed
	   }
	*/
}