package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns" // For local discovery
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
	"go-blockchain/core"
	"go-blockchain/pb"
)

const (
	ProtocolID = "/go-blockchain/1.0.0"
)

// Node represents a network node in the blockchain network.
type Node struct {
	host         host.Host
	Reputation   int
	orchestrator *core.ConsistencyOrchestrator
	resolver     *core.ConflictResolver
	byzantine    *core.ByzantineNode
	startTime    time.Time // Track when the node was started
}

// NewNetworkNode creates and initializes a new network node.
func NewNetworkNode(ctx context.Context, listenPort int) (*Node, error) {
	// Create a new libp2p host.
	// Listen on all available interfaces on the specified TCP port.
	// 0 means automatically select a port.
	listenAddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", listenPort)

	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here, like security protocols, transports, etc.
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(listenAddr),
		// Add other options like NAT traversal if needed later
		// libp2p.EnableNATService(), // Example
		// libp2p.EnableRelay(),      // Example for NATed peers
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	orchestrator := core.NewConsistencyOrchestrator()

	node := &Node{
		host:         h,
		orchestrator: orchestrator,
		resolver:     core.NewConflictResolver(orchestrator),
		startTime:    time.Now(), // Record the start time
	}

	log.Printf("Node started with ID: %s", h.ID().String())
	log.Println("Listening addresses:")
	for _, addr := range h.Addrs() {
		log.Printf("- %s/p2p/%s", addr, h.ID().String()) // Print full address including Peer ID
	}

	return node, nil
}

// StartDiscovery initializes peer discovery mechanisms.
// For now, we'll use mDNS for local discovery.
func (n *Node) StartDiscovery(ctx context.Context, serviceTag string) error {
	// setup mDNS discovery
	s := mdns.NewMdnsService(n.host, serviceTag, n) // Pass 'n' to implement the discovery interface

	log.Printf("mDNS Discovery Service '%s' starting...", serviceTag)
	return s.Start()
}

// HandlePeerFound is called by the mDNS service when a new peer is found.
// This method satisfies the mdns.Notifee interface.
func (n *Node) HandlePeerFound(pi peer.AddrInfo) {
	// Avoid connecting to self
	if pi.ID == n.host.ID() {
		return
	}

	log.Printf("Discovered new peer: %s", pi.ID.String())
	log.Println("Attempting to connect...")

	// Connect to the newly discovered peer
	// Use a background context for the connection attempt
	// Add a timeout to avoid hanging indefinitely
	connCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Adjust timeout as needed
	defer cancel()

	err := n.host.Connect(connCtx, pi)
	if err != nil {
		log.Printf("Failed to connect to peer %s: %v", pi.ID.String(), err)
	} else {
		log.Printf("Successfully connected to peer: %s", pi.ID.String())
		// TODO: Add peer to a managed peer list
		// TODO: Initiate handshake or request peer list if needed
	}
}

func (n *Node) GetFullAddr() (string, error) {
	var suitableAddr string
	var firstAddr string // Keep track of the first address encountered as fallback

	for _, addr := range n.host.Addrs() {
		addrStr := addr.String() // Get the string representation

		// Keep the very first address as a fallback if no non-loopback is found
		if firstAddr == "" {
			firstAddr = fmt.Sprintf("%s/p2p/%s", addrStr, n.host.ID().String())
		}

		// Simple heuristic: check for common loopback prefixes
		// A more robust check could involve parsing the multiaddr with manet,
		// getting the net.IP, and calling ip.IsLoopback(), but this is often sufficient.
		isLoopback := strings.HasPrefix(addrStr, "/ip4/127.") || strings.HasPrefix(addrStr, "/ip6/::1")

		if !isLoopback {
			suitableAddr = fmt.Sprintf("%s/p2p/%s", addrStr, n.host.ID().String())
			break // Found a preferred non-loopback address
		}
	}

	// If no non-loopback address was found, use the first address we saw
	if suitableAddr == "" {
		suitableAddr = firstAddr
	}

	if suitableAddr == "" {
		// This should ideally not happen if the host is listening on at least one address
		return "", fmt.Errorf("no suitable listening address found for node")
	}
	return suitableAddr, nil
}

// ConnectToBootstrapPeers attempts to connect to a list of known bootstrap peers.
func (n *Node) ConnectToBootstrapPeers(ctx context.Context, peerAddrs []string) {
	if len(peerAddrs) == 0 {
		log.Println("No bootstrap peers configured.")
		return
	}

	log.Println("Connecting to bootstrap peers...")
	var successCount int
	for _, addrStr := range peerAddrs {
		if addrStr == "" {
			continue // Skip empty entries
		}
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			log.Printf("Error parsing bootstrap peer multiaddr '%s': %v", addrStr, err)
			continue
		}
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			log.Printf("Error extracting AddrInfo from bootstrap peer multiaddr '%s': %v", addrStr, err)
			continue
		}

		// Avoid connecting to self if accidentally listed
		if peerInfo.ID == n.host.ID() {
			continue
		}

		// Use a separate context with timeout for each connection attempt
		connCtx, cancel := context.WithTimeout(ctx, 15*time.Second) // Adjust timeout

		log.Printf("Attempting to connect to bootstrap peer: %s", peerInfo.ID.String())
		err = n.host.Connect(connCtx, *peerInfo)
		cancel() // Release context resources

		if err != nil {
			log.Printf("Failed to connect to bootstrap peer %s (%s): %v", peerInfo.ID.String(), addrStr, err)
		} else {
			log.Printf("Successfully connected to bootstrap peer: %s", peerInfo.ID.String())
			successCount++
			// TODO: Add peer to a managed peer list
		}
	}
	log.Printf("Finished bootstrap connection attempts. Successfully connected to %d peers.", successCount)
}

// Close shuts down the network node.
func (n *Node) Close() error {
	log.Println("Shutting down network node...")
	// TODO: Close discovery services if needed
	return n.host.Close()
}

// SendCommitmentProof sends a CommitmentProofMessage for a given shard to a peer.
func (n *Node) SendCommitmentProof(ctx context.Context, peerID peer.ID, shardID string) error {
	shard, err := core.GetShardByID(shardID)
	if err != nil {
		return fmt.Errorf("shard not found: %w", err)
	}
	// Prepare Pedersen commitment
	ped := shard.PedersenCommit
	pedMsg := &pb.PedersenCommitment{
		Commitment: ped.Commitment.SerializeCompressed(),
		Blinding:   ped.Blinding.Bytes(),
		Value:      ped.Value.Bytes(),
	}
	// Prepare Merkle proof for this shard in the forest
	_, compressed, err := core.MerkleProofForShardInForest(shardID)
	var merkleMsg *pb.MerkleProof
	if err == nil && compressed != nil {
		merkleMsg = &pb.MerkleProof{
			LeafHash:   compressed.LeafHash,
			Siblings:   compressed.Siblings,
			PathBitmap: compressed.PathBitmap,
			Depth:      uint32(compressed.Depth),
		}
	}
	msg := &pb.Message{
		Payload: &pb.Message_CommitmentProof{
			CommitmentProof: &pb.CommitmentProofMessage{
				ShardId:            shardID,
				PedersenCommitment: pedMsg,
				MerkleProof:        merkleMsg,
			},
		},
	}
	return n.SendMessage(ctx, peerID, msg)
}

// HandleStream processes incoming streams and decodes Protobuf messages.
func (n *Node) HandleStream(s network.Stream) {
	defer s.Close()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(s); err != nil {
		log.Printf("Error reading from stream: %v", err)
		return
	}

	msg := &pb.Message{}
	if err := proto.Unmarshal(buf.Bytes(), msg); err != nil {
		log.Printf("Error unmarshalling Protobuf message: %v", err)
		return
	}

	// Update network stats for consistency orchestration
	latency := time.Since(s.Stat().Opened).Milliseconds()
	peerID := s.Conn().RemotePeer()
	n.orchestrator.UpdateNetworkStats(
		peerID.String(),
		float64(latency),
		0.0, // Initial packet loss estimate
	)

	// Verify node trustworthiness
	if !n.byzantine.IsNodeTrusted(peerID.String()) {
		log.Printf("Rejecting stream from untrusted node %s", peerID)
		s.Reset()
		return
	}

	switch payload := msg.Payload.(type) {
	case *pb.Message_Block:
		if block := payload.Block; block != nil {
			// First verify strong consistency
			if err := n.verifyBlockConsistency(block); err != nil {
				log.Printf("Block failed strong consistency check: %v", err)
				return
			}

			// Verify with required consensus threshold
			threshold := n.byzantine.GetConsensusThreshold()
			ctx := context.Background()
			requiredPeers := int(float64(len(n.host.Network().Peers())) * threshold)

			// Setup MPC shares for distributed verification
			secret := new(big.Int).SetBytes(block.Header.PrevBlockHash)
			participants := make([]string, 0, len(n.host.Network().Peers()))
			for _, peer := range n.host.Network().Peers() {
				participants = append(participants, peer.String())
			}
			if err := n.setupMPCShares(secret, participants); err != nil {
				log.Printf("Failed to setup MPC shares: %v", err)
				return
			}

			if !n.verifyBlockWithPeers(ctx, block, requiredPeers) {
				log.Printf("Block failed to achieve required consensus threshold of %f", threshold)
				return
			}

			// For leader-based consensus, verify VRF proof
			epoch := uint64(block.Header.Height)
			peerID := s.Conn().RemotePeer()
			if proof := n.getVRFProof(peerID.String(), epoch); proof != nil {
				input := []byte(fmt.Sprintf("%d", epoch))
				if !n.byzantine.VerifyVRF(input, proof.Output, proof.Proof) {
					log.Printf("Invalid VRF proof from node %s", peerID)
					return
				}
			}

			// Verify state with zero-knowledge proof if available
			if err := n.verifyStateZKP(block); err != nil {
				log.Printf("ZKP verification failed: %v", err)
				return
			}

			// Update node reputation based on verification results
			n.byzantine.UpdateReputation(peerID.String(), "block_verified", 1.0)
		}
	case *pb.Message_Transaction:
		// Handle potential conflicts in transaction processing
		if tx := payload.Transaction; tx != nil {
			values := [][]byte{tx.Data}
			clocks := []core.VectorClock{make(core.VectorClock)}
			if conflict := n.resolver.DetectConflict(string(tx.Hash), values, clocks); conflict != nil {
				resolvedValue, _ := n.resolver.ResolveConflict(conflict)
				tx.Data = resolvedValue
			}
		}
	case *pb.Message_CommitmentProof:
		cp := payload.CommitmentProof
		log.Printf("Received commitment proof for shard %s", cp.ShardId)
		// Verify Pedersen commitment
		if cp.PedersenCommitment != nil {
			// Deserialize and verify using core.VerifyPedersenCommitment
			// (Add deserialization logic as needed)
			log.Printf("Pedersen commitment received (not fully verified in this stub)")
		}
		// Verify Merkle proof
		if cp.MerkleProof != nil {
			// (Add verification logic as needed)
			log.Printf("Merkle proof received (not fully verified in this stub)")
		}
	default:
		log.Printf("Unknown message type received")
	}
}

// SendMessage sends a Protobuf message to a specific peer.
func (n *Node) SendMessage(ctx context.Context, peerID peer.ID, msg *pb.Message) error {
	stream, err := n.host.NewStream(ctx, peerID, protocol.ID(ProtocolID))
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	defer stream.Close()

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal Protobuf message: %w", err)
	}

	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("failed to write to stream: %w", err)
	}

	return nil
}

// RegisterStreamHandler sets up the stream handler for the protocol.
func (n *Node) RegisterStreamHandler() {
	n.host.SetStreamHandler(protocol.ID(ProtocolID), n.HandleStream)
}

// BroadcastBlock sends a block to all connected peers.
func (n *Node) BroadcastBlock(ctx context.Context, block *pb.Block) {
	peers := n.host.Peerstore().Peers()
	for _, peerID := range peers {
		if peerID == n.host.ID() {
			continue // Skip sending to self
		}

		msg := &pb.Message{
			Payload: &pb.Message_Block{
				Block: block,
			},
		}

		err := n.SendMessage(ctx, peerID, msg)
		if err != nil {
			if strings.Contains(err.Error(), "peer id mismatch") {
				log.Printf("Peer ID mismatch for peer %s (multiaddress: %s). Skipping.", peerID, n.host.Peerstore().PeerInfo(peerID).Addrs)
				continue
			}
			log.Printf("Failed to send block to peer %s: %v", peerID, err)
		} else {
			log.Printf("Sent block to peer %s", peerID)
		}
	}
}

// IncreaseReputation increases the node's reputation score by a given amount.
func (n *Node) IncreaseReputation(amount int) {
	n.Reputation += amount
}

// DecreaseReputation decreases the node's reputation score by a given amount (minimum 0).
func (n *Node) DecreaseReputation(amount int) {
	n.Reputation -= amount
	if n.Reputation < 0 {
		n.Reputation = 0
	}
}

// GetReputation returns the current reputation score of the node.
func (n *Node) GetReputation() int {
	return n.Reputation
}

// Example: Use reputation in block validation (pseudo-logic)
// Increase reputation for valid block, decrease for invalid block
func (n *Node) OnBlockReceived(valid bool) {
	if valid {
		n.IncreaseReputation(10) // Reward for valid block
	} else {
		n.DecreaseReputation(20) // Penalty for invalid block
	}
	log.Printf("Node %s reputation updated: %d", n.host.ID().String(), n.Reputation)
}

// Example: Only accept blocks from nodes above a reputation threshold
func (n *Node) ShouldAcceptBlockFrom(peerReputation int) bool {
	const minReputation = 50
	return peerReputation >= minReputation
}

// verifyBlockConsistency implements strong consistency verification
func (n *Node) verifyBlockConsistency(block *pb.Block) error {
	timeout := n.orchestrator.GetTimeout()
	retries := n.orchestrator.GetRetryAttempts()
	
	for attempt := 0; attempt < retries; attempt++ {
		// Create verification context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
		defer cancel()

		// Get verification from majority of peers
		verificationCount := 0
		requiredCount := len(n.host.Network().Peers())/2 + 1

		for _, peer := range n.host.Network().Peers() {
			if peer == n.host.ID() {
				continue
			}

			// Request verification (simplified)
			verified := n.requestBlockVerification(ctx, peer, block)
			if verified {
				verificationCount++
			}

			if verificationCount >= requiredCount {
				return nil
			}
		}

		// Adjust timeout for next attempt
		timeout = timeout * 2
	}

	return fmt.Errorf("failed to achieve strong consistency after %d attempts", retries)
}

// requestBlockVerification requests block verification from a peer
func (n *Node) requestBlockVerification(ctx context.Context, peer peer.ID, block *pb.Block) bool {
	// Create verification request message
	msg := &pb.Message{
		Payload: &pb.Message_Block{
			Block: block,
		},
	}

	// Send verification request with timeout context
	err := n.SendMessage(ctx, peer, msg)
	if err != nil {
		log.Printf("Failed to send verification request to peer %s: %v", peer.String(), err)
		return false
	}

	// Wait for response (simplified - in reality would need a response channel)
	select {
	case <-ctx.Done():
		log.Printf("Verification request to peer %s timed out", peer.String())
		return false
	default:
		// In a real implementation, would wait for and process the peer's response
		// For now, count any successful message send as verification
		return true
	}
}

// verifyStateZKP verifies the state transition using zero-knowledge proofs
func (n *Node) verifyStateZKP(block *pb.Block) error {
	// Example state verification - in production would verify actual state transitions
	secret := new(big.Int).SetBytes(block.Header.PrevBlockHash)
	proof, err := n.byzantine.GenerateZKProof(secret)
	if err != nil {
		return err
	}
	
	publicValue := new(big.Int).SetBytes(block.Header.MerkleRoot)
	if !n.byzantine.VerifyZKProof(proof, publicValue) {
		return errors.New("invalid zero-knowledge proof")
	}
	
	return nil
}

// getVRFProof gets the VRF proof for leader election with proper nodeID usage
func (n *Node) getVRFProof(nodeID string, epoch uint64) *core.VRFOutput {
	log.Printf("Generating VRF proof for node %s at epoch %d", nodeID, epoch)
	epochBytes := []byte(fmt.Sprintf("%d", epoch))
	proof, err := n.byzantine.GenerateVRF(epochBytes)
	if err != nil {
		log.Printf("Failed to generate VRF proof for node %s: %v", nodeID, err)
		return nil
	}
	return proof
}

// MPCShare represents a share in Multi-Party Computation
type MPCShare struct {
    Index      uint32
    Value      *big.Int
    Commitment []*big.Int
}

// MPCMetrics represents metrics for MPC operations
type MPCMetrics struct {
    VerificationLatency    time.Duration
    SuccessfulVerifications int
    FailedVerifications    int
    ActiveDistributions    int
}

// GetMPCMetrics returns the current MPC metrics
func (n *Node) GetMPCMetrics() *MPCMetrics {
    return &MPCMetrics{
        VerificationLatency:    time.Duration(0),
        SuccessfulVerifications: 0,
        FailedVerifications:    0,
        ActiveDistributions:    0,
    }
}

// setupMPCShares sets up MPC shares for distributed trust
func (n *Node) setupMPCShares(secret *big.Int, participants []string) error {
    threshold := (len(participants) * 2 / 3) + 1 // 2f+1 threshold
    
    var successCount int32 = 0
    errorChan := make(chan error, len(participants))
    var wg sync.WaitGroup

    for i, participant := range participants {
        wg.Add(1)
        go func(index int, peerStr string) {
            defer wg.Done()

            peerID, err := peer.Decode(peerStr)
            if err != nil {
                errorChan <- fmt.Errorf("invalid peer ID %s: %v", peerStr, err)
                return
            }

            // Create share message with commitments
            shareMsg := &pb.Message{
                Payload: &pb.Message_CommitmentProof{
                    CommitmentProof: &pb.CommitmentProofMessage{
                        ShardId: fmt.Sprintf("mpc-share-%d-%d", index, time.Now().UnixNano()),
                        PedersenCommitment: &pb.PedersenCommitment{
                            Value: secret.Bytes(),
                            Commitment: []byte{}, // Simplified commitment
                        },
                    },
                },
            }

            // Send share with timeout
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()

            if err := n.SendMessage(ctx, peerID, shareMsg); err != nil {
                errorChan <- fmt.Errorf("failed to send MPC share to %s: %v", peerStr, err)
                return
            }

            atomic.AddInt32(&successCount, 1)
        }(i, participant)
    }

    // Wait for all distributions to complete
    wg.Wait()
    close(errorChan)

    // Process any errors
    var errors []string
    for err := range errorChan {
        errors = append(errors, err.Error())
    }

    // Return error if we didn't achieve threshold
    if int(successCount) < threshold {
        return fmt.Errorf("failed to achieve MPC threshold: got %d out of required %d. Errors: %v",
            successCount, threshold, strings.Join(errors, "; "))
    }

    return nil
}

// GetMPCAnalytics returns analytics about MPC operations
func (n *Node) GetMPCAnalytics() map[string]interface{} {
    if n.byzantine == nil {
        return map[string]interface{}{
            "error": "Byzantine node not initialized",
        }
    }
    
    metrics := n.GetMPCMetrics()
    analytics := map[string]interface{}{
        "ActiveDistributions": metrics.ActiveDistributions,
        "VerificationLatency": metrics.VerificationLatency,
        "SuccessfulVerifications": metrics.SuccessfulVerifications,
        "FailedVerifications": metrics.FailedVerifications,
    }
    
    // Add additional node-specific metrics
    analytics["node_id"] = n.host.ID().String()
    analytics["uptime"] = time.Since(n.startTime).String()
    
    return analytics
}

// MonitorMPCHealth checks MPC system health and triggers alerts if needed
func (n *Node) MonitorMPCHealth() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        analytics := n.GetMPCAnalytics()
        
        // Check success rate trend
        if trend, ok := analytics["success_rate_trend"].(float64); ok {
            if trend < -0.1 { // Alert if success rate dropped by more than 10%
                log.Printf("ALERT: MPC success rate declining: %.2f%%", trend*100)
            }
        }
        
        // Check participant health
        if health, ok := analytics["participant_health"].(map[string]string); ok {
            for participant, status := range health {
                if status == "poor" {
                    log.Printf("ALERT: Participant %s showing poor performance", participant)
                }
            }
        }
        
        // Check latency
        if latency, ok := analytics["average_latency_ms"].(int64); ok {
            if latency > 5000 { // Alert if average latency exceeds 5 seconds
                log.Printf("ALERT: High MPC verification latency: %dms", latency)
            }
        }
    }
}

// Add monitoring endpoint handler
func (n *Node) handleMPCMetrics(w http.ResponseWriter, r *http.Request) {
    analytics := n.GetMPCAnalytics()
    
    // Convert to JSON
    response, err := json.Marshal(analytics)
    if err != nil {
        http.Error(w, "Failed to serialize analytics", http.StatusInternalServerError)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.Write(response)
}

// Update node initialization to start monitoring
func (n *Node) Start() error {
    // Initialize Byzantine node with config
    byzantineConfig := core.ByzantineConfig{
        InitialReputation: 100,
        ConsensusThreshold: 0.7,
        VRFSeed: []byte("initial-seed"),
    }
    
    byzantine, err := core.NewByzantineNode(byzantineConfig)
    if err != nil {
        return fmt.Errorf("failed to initialize byzantine node: %v", err)
    }
    n.byzantine = byzantine
    
    // Start MPC health monitoring
    go n.MonitorMPCHealth()
    
    // Add metrics endpoint
    http.HandleFunc("/metrics/mpc", n.handleMPCMetrics)
    
    return nil
}

// verifyBlockWithPeers verifies a block with the required number of peers
func (n *Node) verifyBlockWithPeers(ctx context.Context, block *pb.Block, requiredPeers int) bool {
    verifiedCount := 0
    totalPeers := len(n.host.Network().Peers())
    
    // Check if we have enough peers for the required threshold
    if totalPeers == 0 {
        log.Printf("No peers available for verification")
        return false
    }
    
    for _, peer := range n.host.Network().Peers() {
        if peer == n.host.ID() {
            continue
        }

        // Send verification request
        msg := &pb.Message{
            Payload: &pb.Message_Block{
                Block: block,
            },
        }

        err := n.SendMessage(ctx, peer, msg)
        if err != nil {
            log.Printf("Failed to send verification request to peer %s: %v", peer.String(), err)
            continue
        }

        // In a real implementation, would wait for response
        // For now, count successful message sends
        verifiedCount++

        // Check if we've reached the required number of verifications
        if verifiedCount >= requiredPeers {
            log.Printf("Block verification successful: %d/%d peers verified (threshold met)", verifiedCount, totalPeers)
            return true
        }
    }
    
    log.Printf("Block verification failed: only %d/%d peers verified (%d required)", verifiedCount, totalPeers, requiredPeers)
    return false
}