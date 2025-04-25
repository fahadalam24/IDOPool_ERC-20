package network

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns" // For local discovery
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"
	"go-blockchain/pb"
)

const (
	ProtocolID = "/go-blockchain/1.0.0"
)

// Node represents a network node in the blockchain network.
type Node struct {
	host       host.Host // The libp2p host instance
	Reputation int       // Reputation score for the node
	// TODO: Add channels for communication with core blockchain logic
	// TODO: Add peerstore management
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

	node := &Node{
		host: h,
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

	switch payload := msg.Payload.(type) {
	case *pb.Message_Block:
		// Process the block payload
		if block := payload.Block; block != nil {
			log.Printf("Received block with height %d", block.Header.Height)
			// TODO: Validate and add the block to the blockchain
		}
	case *pb.Message_Transaction:
		log.Printf("Received transaction from peer: %s", s.Conn().RemotePeer())
		// TODO: Pass the transaction to the blockchain logic
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