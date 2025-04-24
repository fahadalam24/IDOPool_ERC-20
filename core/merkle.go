package core

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt" // Added fmt for potential error formatting if needed later
)

// Define a constant for the hash of an empty set of transactions.
// This makes the handling of the zero-transaction case explicit and consistent.
var emptyMerkleRootHash = sha256.Sum256([]byte{})

// MerkleNode represents a node in the Merkle Tree.
// For leaf nodes, Data is the hash of the transaction.
// For internal nodes, Data is the hash of the concatenation of its children's Data.
type MerkleNode struct {
	Left  *MerkleNode // Pointer to the left child node
	Right *MerkleNode // Pointer to the right child node
	Data  []byte      // Hash data of the node
}

// MerkleTree represents the complete Merkle Tree, holding the root node.
type MerkleTree struct {
	RootNode *MerkleNode // Pointer to the root node of the tree
}

// NewMerkleLeafNode creates a new Merkle tree leaf node from a hash.
// It's assumed the input 'data' is already the hash we want to use for the leaf.
func NewMerkleLeafNode(data []byte) *MerkleNode {
	// Basic validation: Ensure data is not nil or empty?
	// Or trust the caller provides valid hashes. Let's trust for now.
	node := &MerkleNode{
		Data:  data,
		Left:  nil, // Leaf nodes have no children
		Right: nil,
	}
	return node
}

// NewMerkleNode creates a new internal Merkle tree node from two child nodes.
// It calculates the hash based on the children's hashes.
// Handles the case where the right child might be nil (implicitly meaning it should
// be treated as a duplicate of the left child for hashing) by relying on the
// tree construction logic to provide the correct duplicated node reference.
func NewMerkleNode(left, right *MerkleNode) (*MerkleNode, error) {
	// --- Input Validation ---
	// An internal node MUST have a left child in standard construction.
	if left == nil || len(left.Data) == 0 {
		return nil, errors.New("cannot create internal merkle node without a left child")
	}
	// The construction algorithm ensures 'right' is a valid node (potentially a duplicate
	// of 'left' if needed), so we don't expect 'right' to be nil here *unless*
	// the tree had only one node to begin with, which is handled before calling this.
	// However, a defensive check is good.
	if right == nil || len(right.Data) == 0 {
		// This technically shouldn't happen if NewMerkleTree's logic is correct,
		// as it duplicates the node before calling NewMerkleNode.
		return nil, errors.New("internal merkle node creation called with nil right child unexpectedly")
		// If we wanted to be extremely resilient, we could duplicate here,
		// but it suggests an error elsewhere:
		// right = left
	}

	// --- Hash Calculation ---
	// Concatenate the hashes of the left and right children.
	// The order (left then right) is important.
	headers := bytes.Join([][]byte{left.Data, right.Data}, []byte{})

	// Calculate the hash of the concatenated hashes.
	hash := sha256.Sum256(headers)

	// --- Create Node ---
	node := &MerkleNode{
		Left:  left,
		Right: right,
		Data:  hash[:], // Assign the hash slice to the node's data
	}

	return node, nil
}

// NewMerkleTree creates a new Merkle Tree from a slice of hashes (data).
// It handles zero, one, even, and odd numbers of input hashes correctly.
func NewMerkleTree(data [][]byte) (*MerkleTree, error) {
	// --- Case 1: Zero Transactions ---
	if len(data) == 0 {
		// Return a tree whose root node contains the predefined empty hash.
		// Note: This root is technically a leaf node in this specific empty tree case.
		rootNode := NewMerkleLeafNode(emptyMerkleRootHash[:])
		return &MerkleTree{RootNode: rootNode}, nil
	}

	// --- Create Initial Leaf Nodes ---
	var nodes []*MerkleNode
	for _, datum := range data {
		// Input 'datum' is assumed to be a transaction hash already.
		if len(datum) == 0 {
			// Handle potentially invalid input hash? Return error or skip?
			// Let's return an error for robustness.
			return nil, errors.New("invalid zero-length hash provided for merkle leaf")
		}
		node := NewMerkleLeafNode(datum)
		nodes = append(nodes, node)
	}

	// --- Case 2: One Transaction ---
	// If there was only one transaction, the 'nodes' slice has one element.
	// The loop 'for len(nodes) > 1' below will not execute.
	// The single leaf node is itself the root of the tree.
	if len(nodes) == 1 {
		return &MerkleTree{RootNode: nodes[0]}, nil
	}

	// --- Build the Tree Level by Level (Handles Even and Odd Cases) ---
	level := 1 // For potential debugging/logging
	for len(nodes) > 1 {
		// --- Case 4: Odd Number of Nodes at Current Level ---
		// If the current level has an odd number of nodes, duplicate the last one.
		if len(nodes)%2 != 0 {
			// Append the last node again to the slice. This doesn't deep copy the node,
			// just its reference, which is sufficient for the hashing process.
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		// --- Process Pairs to Create the Next Level Up ---
		var newLevel []*MerkleNode
		// --- Case 3: Even Number of Nodes (implicit after odd handling) ---
		for i := 0; i < len(nodes); i += 2 {
			// We are guaranteed to have pairs because of the odd check above.
			left := nodes[i]
			right := nodes[i+1] // This index is safe due to the odd check.

			node, err := NewMerkleNode(left, right)
			if err != nil {
				// Propagate error from node creation
				return nil, fmt.Errorf("failed to create internal node at level %d, index %d: %w", level, i/2, err)
			}
			newLevel = append(newLevel, node)
		}
		nodes = newLevel // Move up to the next level
		level++
	}

	// After the loop, 'nodes' should contain exactly one node: the root.
	if len(nodes) != 1 {
		// This condition indicates a logical error in the tree construction loop.
		return nil, fmt.Errorf("merkle tree construction failed, ended with %d nodes instead of 1", len(nodes))
	}

	tree := MerkleTree{RootNode: nodes[0]}
	return &tree, nil
}