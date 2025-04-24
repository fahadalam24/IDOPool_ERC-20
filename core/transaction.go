package core

import (
	"crypto/sha256"
	// "encoding/json" // No longer needed for hashing
	"go-blockchain/pb" // Import the generated package

	"google.golang.org/protobuf/proto" // Import the protobuf library
)

// Transaction represents a simple transaction in the blockchain.
type Transaction struct {
	Data []byte `json:"data"` // Keep JSON tag for potential API/RPC use later
	hash []byte // Cached hash
}

// Hash calculates and returns the SHA256 hash of the transaction.
// Uses Protobuf marshalling for a deterministic representation.
func (t *Transaction) Hash() ([]byte, error) {
	if t.hash != nil {
		return t.hash, nil
	}

	// Create the corresponding Protobuf message
	txProto := &pb.Transaction{
		Data: t.Data,
		// Ensure all fields used for hashing are included here if you add more
	}

	txBytes, err := proto.Marshal(txProto) // Use proto.Marshal
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(txBytes)
	t.hash = hash[:] // Cache the hash
	return t.hash, nil
}

// NewTransaction creates a new transaction with the given data.
func NewTransaction(data []byte) *Transaction {
	tx := &Transaction{Data: data}
	// Optionally calculate hash on creation? Or keep it lazy. Lazy is fine.
	return tx
}

// GetHash returns the cached hash without recalculating.
// Useful if you are sure Hash() has been called before.
func (t *Transaction) GetHash() []byte {
	// Consider calculating if nil, or requiring Hash() to be called first.
	// For simplicity, let's assume Hash() might be called separately first.
	return t.hash
}