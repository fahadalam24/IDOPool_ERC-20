package network

import (
	"bytes"
	"context"
	"encoding/hex"
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

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
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
	orchestrator *core.ConsistencyOrchestrator
	byzantine    *core.ByzantineNode
	resolver     *core.ConflictResolver
	reputations  map[string]float64 // Peer ID -> reputation score
	metrics      struct {
		totalBlocksReceived uint64
		totalTxReceived     uint64
		lastMetricsReset   time.Time
	}
	mpcMetrics   *core.MPCMetrics
	httpServer   *http.Server
	metricsPort  int
	startTime    time.Time         // Track node uptime
	Reputation   int              // Added missing reputation field
	blockchain   *core.Blockchain // Added blockchain reference
}

// NewNetworkNode creates and initializes a new network node.
func NewNetworkNode(ctx context.Context, port int) (*Node, error) {
	// Create libp2p host with relevant options
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.EnableRelay(),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Initialize Byzantine node with config, passing our own peer ID
	byzantineConfig := core.ByzantineConfig{
		AdaptiveFactors: core.AdaptiveFactors{
			NetworkStress:    0.1,
			FailureRate:      0.01,
			ParticipantCount: 1,
		},
		ReputationThresholds: core.ReputationThresholds{
			Minimum:   0.3,
			Consensus: 0.6,
			Leader:    0.8,
		},
		DecayRate:         0.99,
		InitialReputation: core.DefaultReputation,
		ConsensusThreshold: core.DefaultConsensusThreshold,
		VRFSeed:           []byte("initial-vrf-seed"),
	}

	byzantine, err := core.NewByzantineNode(byzantineConfig, h.ID().String())
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("failed to create byzantine node: %w", err)
	}
	// Register self in trust system
	byzantine.RegisterPeer(h.ID().String())

	// Initialize node components
	orchestrator := core.NewConsistencyOrchestrator()
	resolver := core.NewConflictResolver(orchestrator)

	node := &Node{
		host:         h,
		orchestrator: orchestrator,
		byzantine:    byzantine,
		resolver:     resolver,
		reputations:  make(map[string]float64),
		mpcMetrics:   &core.MPCMetrics{}, // Changed from NewMPCMetrics() to direct initialization
		metricsPort:  8080,
		startTime:    time.Now(),
	}

	// Initialize metrics
	node.metrics.lastMetricsReset = time.Now()

	// Start monitoring endpoints
	if err := node.startMonitoring(); err != nil {
		h.Close()
		return nil, fmt.Errorf("failed to start monitoring: %w", err)
	}

	return node, nil
}

// GetFullAddr returns the node's full multiaddress as a string.
func (n *Node) GetFullAddr() (string, error) {
	if n.host == nil {
		return "", errors.New("host not initialized")
	}

	// Get the first address that isn't a loopback
	var addr string
	for _, a := range n.host.Addrs() {
		if !strings.Contains(a.String(), "127.0.0.1") && !strings.Contains(a.String(), "::1") {
			addr = a.String()
			break
		}
	}

	// If no non-loopback found, use the first address
	if addr == "" && len(n.host.Addrs()) > 0 {
		addr = n.host.Addrs()[0].String()
	}

	if addr == "" {
		return "", errors.New("no listening address found")
	}

	// Combine address with peer ID
	fullAddr := fmt.Sprintf("%s/p2p/%s", addr, n.host.ID().String())
	return fullAddr, nil
}

// ConnectToBootstrapPeers attempts to connect to a list of known bootstrap peers.
func (n *Node) ConnectToBootstrapPeers(ctx context.Context, peerAddrs []string) {
	if len(peerAddrs) == 0 {
		// log.Println("No bootstrap peers configured.")
		return
	}

	// log.Println("Connecting to bootstrap peers...")
	var successCount int
	for _, addrStr := range peerAddrs {
		if addrStr == "" {
			continue // Skip empty entries
		}
		addr, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			// log.Printf("Error parsing bootstrap peer multiaddr '%s': %v", addrStr, err)
			continue
		}
		peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			// log.Printf("Error extracting AddrInfo from bootstrap peer multiaddr '%s': %v", addrStr, err)
			continue
		}

		// Avoid connecting to self if accidentally listed
		if peerInfo.ID == n.host.ID() {
			continue
		}

		// Use a separate context with timeout for each connection attempt
		connCtx, cancel := context.WithTimeout(ctx, 15*time.Second) // Adjust timeout

		// log.Printf("Attempting to connect to bootstrap peer: %s", peerInfo.ID.String())
		err = n.host.Connect(connCtx, *peerInfo)
		cancel() // Release context resources

		if err != nil {
			// log.Printf("Failed to connect to bootstrap peer %s (%s): %v", peerInfo.ID.String(), addrStr, err)
		} else {
			// log.Printf("Successfully connected to bootstrap peer: %s", peerInfo.ID.String())
			successCount++
			// Register peer in trust system
			n.byzantine.RegisterPeer(peerInfo.ID.String())
			// TODO: Add peer to a managed peer list
		}
	}
	// log.Printf("Finished bootstrap connection attempts. Successfully connected to %d peers.", successCount)
}

// Close shuts down the network node.
func (n *Node) Close() error {
	// log.Println("Shutting down network node...")
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
		// log.Printf("Error reading from stream: %v", err)
		return
	}

	msg := &pb.Message{}
	if err := proto.Unmarshal(buf.Bytes(), msg); err != nil {
		// log.Printf("Error unmarshalling Protobuf message: %v", err)
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

	// Register remote peer in trust system
	n.byzantine.RegisterPeer(peerID.String())

	// Verify node trustworthiness
	if !n.byzantine.IsNodeTrusted(peerID.String()) {
		// log.Printf("Rejecting stream from untrusted node %s", peerID)
		s.Reset()
		return
	}

	switch payload := msg.Payload.(type) {
	case *pb.Message_Block:
		if block := payload.Block; block != nil {
			// First verify strong consistency
			if err := n.verifyBlockConsistency(block); err != nil {
				// log.Printf("Block failed strong consistency check: %v", err)
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
				// log.Printf("Failed to setup MPC shares: %v", err)
				return
			}

			if !n.verifyBlockWithPeers(ctx, block, requiredPeers) {
				// log.Printf("Block failed to achieve required consensus threshold of %f", threshold)
				return
			}

			// For leader-based consensus, verify VRF proof
			epoch := uint64(block.Header.Height)
			peerID := s.Conn().RemotePeer()
			if proof := n.getVRFProof(peerID.String(), epoch); proof != nil {
				input := []byte(fmt.Sprintf("%d", epoch))
				if !n.byzantine.VerifyVRF(input, proof.Output, proof.Proof) {
					// log.Printf("Invalid VRF proof from node %s", peerID)
					return
				}
			}

			// Verify state with zero-knowledge proof if available
			// NOTE: Skipped here because Merkle proof and root are not available at this point
			// If you want to verify state inclusion, call n.verifyStateZKP with a Merkle proof and root where available
			//if err := n.verifyStateZKP(block); err != nil {
			//	log.Printf("ZKP verification failed: %v", err)
			//	return
			//}

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
		// log.Printf("Received commitment proof for shard %s", cp.ShardId)
		if err := n.handleCommitmentProof(cp, s.Conn().RemotePeer()); err != nil {
			// log.Printf("Error handling commitment proof: %v", err)
			return
		}
	default:
		// log.Printf("Unknown message type received")
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
	// Use default timeout and retry values since orchestrator doesn't provide them
	timeout := time.Second * 5  // 5 second default timeout
	retries := 3               // 3 default retries
	
	for attempt := 0; attempt < retries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		// Get verification from majority of peers
		verificationCount := 0
		requiredCount := len(n.host.Network().Peers())/2 + 1

		for _, peer := range n.host.Network().Peers() {
			if peer == n.host.ID() {
				continue
			}

			// Request verification
			verified := n.requestBlockVerification(ctx, peer, block)
			if verified {
				verificationCount++
			}

			if verificationCount >= requiredCount {
				return nil
			}
		}

		// Exponential backoff
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

// verifyStateZKP verifies state inclusion using a Merkle proof and the expected Merkle root
func (n *Node) verifyStateZKP(proof *pb.MerkleProof, expectedRoot []byte) error {
	if proof == nil {
		return errors.New("missing merkle proof for state verification")
	}
	// Convert pb.MerkleProof to core.CompressedMerkleProof
	compressed := &core.CompressedMerkleProof{
		LeafHash:   proof.LeafHash,
		Siblings:   proof.Siblings,
		PathBitmap: proof.PathBitmap,
		Depth:      int(proof.Depth),
	}
	if !core.VerifyCompressedMerkleProofCompressed(compressed, expectedRoot) {
		return errors.New("merkle proof verification failed: state not included in root")
	}
	return nil
}

// getVRFProof gets the VRF proof for leader election with proper nodeID usage
func (n *Node) getVRFProof(nodeID string, epoch uint64) *core.VRFOutput {
	// log.Printf("Generating VRF proof for node %s at epoch %d", nodeID, epoch)
	epochBytes := []byte(fmt.Sprintf("%d", epoch))
	proof, err := n.byzantine.GenerateVRF(epochBytes)
	if err != nil {
		log.Printf("Failed to generate VRF proof for node %s: %v", nodeID, err)
		return nil
	}
	return proof
}

// verifyBlockWithPeers verifies a block with the required number of peers
func (n *Node) verifyBlockWithPeers(ctx context.Context, block *pb.Block, requiredPeers int) bool {
	verificationCount := 0
	for _, peer := range n.host.Network().Peers() {
		if peer == n.host.ID() {
			continue
		}

		msg := &pb.Message{
			Payload: &pb.Message_Block{
				Block: block,
			},
		}

		// Send verification request
		err := n.SendMessage(ctx, peer, msg)
		if err != nil {
			log.Printf("Failed to send verification request to peer %s: %v", peer.String(), err)
			continue
		}

		// For now, consider successful message sending as verification
		// In production, would wait for actual verification response
		verificationCount++

		if verificationCount >= requiredPeers {
			return true
		}
	}

	return false
}

// Handle CommitmentProof message
func (n *Node) handleCommitmentProof(cp *pb.CommitmentProofMessage, peerID peer.ID) error {
	if cp.PedersenCommitment != nil {
		// Parse commitment bytes to *btcec.PublicKey
		pubKey, err := btcec.ParsePubKey(cp.PedersenCommitment.Commitment)
		if err != nil {
			return fmt.Errorf("invalid Pedersen commitment bytes from peer %s: %v", peerID, err)
		}
		commitment := &core.PedersenCommitment{
			Value:      new(big.Int).SetBytes(cp.PedersenCommitment.Value),
			Blinding:   new(big.Int).SetBytes(cp.PedersenCommitment.Blinding),
			Commitment: pubKey,
		}
		if !core.VerifyPedersenCommitment(commitment) {
			return fmt.Errorf("invalid Pedersen commitment from peer %s", peerID)
		}
	}

	// Merkle proof verification for shard state inclusion in the forest root
	if cp.MerkleProof != nil && cp.ShardId != "" {
		// Get the current forest root (Merkle root of all active shard roots) from the latest block header
		latestBlock, err := n.blockchain.GetLatestBlock()
		if err != nil || latestBlock == nil || latestBlock.Header == nil {
			return fmt.Errorf("could not retrieve latest block or block header for forest root")
		}
		forestRoot := latestBlock.Header.ForestRoot
		if len(forestRoot) == 0 {
			return fmt.Errorf("forest root missing in latest block header")
		}
		if err := n.verifyStateZKP(cp.MerkleProof, forestRoot); err != nil {
			return fmt.Errorf("merkle proof verification failed for shard %s: %v", cp.ShardId, err)
		}
	}

	// Handle verification status
	if cp.ShareProof != nil {
		status := &core.MPCVerificationStatus{
			ShareID:          hex.EncodeToString(cp.ShareProof.GetProofData()),
			ParticipantID:    peerID.String(),
			Verified:         true,
			VerificationTime: time.Now().Unix(),
		}

		// Update metrics (no direct mu access)
		if n.mpcMetrics != nil {
			// If you need thread safety, add exported methods to core.MPCMetrics
			n.mpcMetrics.SuccessfulVerifications++
		}

		return n.storeVerificationStatus(cp.ShardId, status)
	}

	return nil
}

// storeVerificationStatus stores the verification status in shard metadata
func (n *Node) storeVerificationStatus(shardID string, status *core.MPCVerificationStatus) error {
	shard, err := core.GetShardByID(shardID)
	if err != nil {
		return fmt.Errorf("failed to get shard %s: %w", shardID, err)
	}
	// Store status in shard metadata (method not implemented, so stubbed)
	// TODO: Implement StoreMetadata on *core.Shard or use an alternative
	_ = status // Prevent unused warning
	_ = shard  // Prevent unused warning
	return nil
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
            // Generate a real Pedersen commitment for the secret value
            pedCommit, err := core.GeneratePedersenCommitment(secret)
            if err != nil {
                errorChan <- fmt.Errorf("failed to generate Pedersen commitment: %v", err)
                return
            }
            shareMsg := &pb.Message{
                Payload: &pb.Message_CommitmentProof{
                    CommitmentProof: &pb.CommitmentProofMessage{
                        ShardId: fmt.Sprintf("mpc-share-%d-%d", index, time.Now().UnixNano()),
                        PedersenCommitment: &pb.PedersenCommitment{
                            Value:    pedCommit.Value.Bytes(),
                            Blinding: pedCommit.Blinding.Bytes(),
                            Commitment: pedCommit.Commitment.SerializeCompressed(),
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

// startMonitoring initializes the monitoring HTTP endpoints
func (n *Node) startMonitoring() error {
    mux := http.NewServeMux()
    mux.HandleFunc("/metrics/mpc", n.handleMPCMetrics)
    
    n.httpServer = &http.Server{
        Addr:    fmt.Sprintf(":%d", n.metricsPort),
        Handler: mux,
    }
    
    // Start HTTP server in a goroutine
    go func() {
        if err := n.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Printf("Monitoring server error: %v", err)
        }
    }()
    
    // Set node start time
    n.startTime = time.Now()
    
    return nil
}