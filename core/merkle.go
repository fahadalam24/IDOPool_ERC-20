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

// MerkleProof represents a standard Merkle proof for a leaf.
type MerkleProof struct {
	LeafHash []byte   // The hash of the leaf being proven
	Siblings [][]byte // The sibling hashes along the path to the root
	PathBits []bool   // true if the leaf is a left child at each level, false if right
}

// GenerateMerkleProof generates a Merkle proof for a given leaf index.
func GenerateMerkleProof(tree *MerkleTree, leafIndex int, leaves [][]byte) *MerkleProof {
	if tree == nil || tree.RootNode == nil || leafIndex < 0 || leafIndex >= len(leaves) {
		return nil
	}
	var siblings [][]byte
	var pathBits []bool
	numLeaves := len(leaves)
	index := leafIndex
	levelNodes := make([][]byte, numLeaves)
	copy(levelNodes, leaves)
	for numLeaves > 1 {
		var nextLevel [][]byte
		for i := 0; i < numLeaves; i += 2 {
			left := levelNodes[i]
			var right []byte
			if i+1 < numLeaves {
				right = levelNodes[i+1]
			} else {
				right = left
			}
			hash := sha256.Sum256(append(left, right...))
			nextLevel = append(nextLevel, hash[:])
		}
		// Record sibling for the current index
		if index%2 == 0 {
			// Sibling is to the right
			if index+1 < numLeaves {
				siblings = append(siblings, levelNodes[index+1])
			} else {
				siblings = append(siblings, levelNodes[index])
			}
			pathBits = append(pathBits, true)
		} else {
			// Sibling is to the left
			siblings = append(siblings, levelNodes[index-1])
			pathBits = append(pathBits, false)
		}
		index /= 2
		levelNodes = nextLevel
		numLeaves = len(levelNodes)
	}
	return &MerkleProof{
		LeafHash: leaves[leafIndex],
		Siblings: siblings,
		PathBits: pathBits,
	}
}

// CompressedMerkleProof uses a bitmap for the path.
type CompressedMerkleProof struct {
	LeafHash   []byte
	Siblings   [][]byte
	PathBitmap uint64 // Up to 64 levels; 1=left, 0=right
	Depth      int
}

func CompressMerkleProof(proof *MerkleProof) *CompressedMerkleProof {
	var bitmap uint64
	for i, bit := range proof.PathBits {
		if bit {
			bitmap |= (1 << i)
		}
	}
	return &CompressedMerkleProof{
		LeafHash:   proof.LeafHash,
		Siblings:   proof.Siblings,
		PathBitmap: bitmap,
		Depth:      len(proof.PathBits),
	}
}

func VerifyCompressedMerkleProofCompressed(proof *CompressedMerkleProof, root []byte) bool {
	if proof == nil || len(proof.Siblings) == 0 {
		return false
	}
	current := proof.LeafHash
	for i := 0; i < proof.Depth; i++ {
		if (proof.PathBitmap & (1 << i)) != 0 {
			// left
			combined := append(current, proof.Siblings[i]...)
			h := sha256.Sum256(combined)
			current = h[:]
		} else {
			// right
			combined := append(proof.Siblings[i], current...)
			h := sha256.Sum256(combined)
			current = h[:]
		}
	}
	return bytes.Equal(current, root)
}

// MerkleProofForShardInForest generates a Merkle proof for a shard's root in the forest root.
func MerkleProofForShardInForest(shardID string) (*MerkleProof, *CompressedMerkleProof, error) {
	// Collect all active shard roots in order
	var shardIDs []string
	var shardRoots [][]byte
	for id, shard := range ShardRegistry {
		if len(shard.MerkleRoot) > 0 {
			shardIDs = append(shardIDs, id)
			shardRoots = append(shardRoots, shard.MerkleRoot)
		}
	}
	// Find the index of the requested shard
	var idx int = -1
	for i, id := range shardIDs {
		if id == shardID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, nil, fmt.Errorf("shard %s not found or has no Merkle root", shardID)
	}
	// Build the forest Merkle tree
	forestTree, err := NewMerkleTree(shardRoots)
	if err != nil {
		return nil, nil, err
	}
	// Generate and compress the Merkle proof for this shard's root
	proof := GenerateMerkleProof(forestTree, idx, shardRoots)
	compressed := CompressMerkleProof(proof)
	return proof, compressed, nil
}

// Test function for Merkle proof-of-inclusion for a shard in the forest root
func TestMerkleProofOfShardInForest() {
	// Simulate three shards with dummy roots
	shardA := CreateShard("shardA", "")
	shardB := CreateShard("shardB", "")
	shardC := CreateShard("shardC", "")
	shardA.MerkleRoot = []byte("rootA")
	shardB.MerkleRoot = []byte("rootB")
	shardC.MerkleRoot = []byte("rootC")

	// Build the forest root
	forestTree, err := NewMerkleTree([][]byte{shardA.MerkleRoot, shardB.MerkleRoot, shardC.MerkleRoot})
	if err != nil {
		fmt.Println("Forest tree error:", err)
		return
	}
	forestRoot := forestTree.RootNode.Data
	fmt.Printf("Forest root: %x\n", forestRoot)

	// Generate and verify proof for shardB
	_, compressed, err := MerkleProofForShardInForest("shardB")
	if err != nil {
		fmt.Println("Proof error:", err)
		return
	}
	valid := VerifyCompressedMerkleProofCompressed(compressed, forestRoot)
	fmt.Println("Compressed proof for shardB valid:", valid)
}