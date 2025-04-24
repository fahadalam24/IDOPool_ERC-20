package core

import (
	"crypto/sha256"
	"encoding/json"
)

// Transaction represents a simple transaction in the blockchain.
// For now, it just holds arbitrary data. We'll add sender, receiver, signature later.
type Transaction struct {
	Data []byte `json:"data"` // Arbitrary data for the transaction
	// TODO: Add Sender, Recipient, Amount, Nonce, Signature, etc. later
}

// Hash calculates and returns the SHA256 hash of the transaction.
// Uses JSON marshalling for a deterministic representation.
func (t *Transaction) Hash() ([]byte, error) {
	txBytes, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(txBytes)
	return hash[:], nil // Return slice of the hash array
}

// NewTransaction creates a new transaction with the given data.
func NewTransaction(data []byte) *Transaction {
	return &Transaction{Data: data}
}