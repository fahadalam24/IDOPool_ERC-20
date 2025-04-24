package network

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns" // For local discovery
	"github.com/multiformats/go-multiaddr"
)

// NetworkNode represents a node in the P2P network.
type NetworkNode struct {
	host host.Host // The libp2p host instance
	// TODO: Add channels for communication with core blockchain logic
	// TODO: Add peerstore management
}

// NewNetworkNode creates and initializes a new network node.
func NewNetworkNode(ctx context.Context, listenPort int) (*NetworkNode, error) {
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

	node := &NetworkNode{
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
func (n *NetworkNode) StartDiscovery(ctx context.Context, serviceTag string) error {
	// setup mDNS discovery
	s := mdns.NewMdnsService(n.host, serviceTag, n) // Pass 'n' to implement the discovery interface

	log.Printf("mDNS Discovery Service '%s' starting...", serviceTag)
	return s.Start()
}

// HandlePeerFound is called by the mDNS service when a new peer is found.
// This method satisfies the mdns.Notifee interface.
func (n *NetworkNode) HandlePeerFound(pi peer.AddrInfo) {
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

func (n *NetworkNode) GetFullAddr() (string, error) {
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
func (n *NetworkNode) ConnectToBootstrapPeers(ctx context.Context, peerAddrs []string) {
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
func (n *NetworkNode) Close() error {
	log.Println("Shutting down network node...")
	// TODO: Close discovery services if needed
	return n.host.Close()
}