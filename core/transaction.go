package core

import (
	"crypto/sha256"
	"fmt"
	"go-blockchain/pb"
	
	"google.golang.org/protobuf/proto"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// Transaction represents a complete transaction in the blockchain.
type Transaction struct {
	Data           []byte `json:"data"`           // Transaction data
	SenderPubKey   []byte `json:"senderPubKey"`   // Public key of the sender
	RecipientAddr  []byte `json:"recipientAddr"`  // Address of the recipient
	Amount         uint64 `json:"amount"`         // Transaction amount
	Nonce         uint64 `json:"nonce"`         // Transaction nonce for replay protection
	Signature     []byte `json:"signature"`      // Transaction signature
	hash          []byte // Cached hash
}

// NewTransaction creates a new transaction with the given data and recipient.
func NewTransaction(data []byte) *Transaction {
	return &Transaction{
		Data:          data,
		Nonce:         0, // Will be set when adding to mempool
		Amount:        0, // Default for data-only transactions
	}
}

// NewValueTransaction creates a new transaction with amount transfer.
func NewValueTransaction(recipientAddr []byte, amount uint64, nonce uint64) *Transaction {
	return &Transaction{
		RecipientAddr: recipientAddr,
		Amount:       amount,
		Nonce:       nonce,
	}
}

// Sign signs the transaction with the given private key.
func (t *Transaction) Sign(privateKey []byte) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("invalid private key length")
	}

	// Create deterministic representation for signing
	txToSign := &pb.Transaction{
		Data:          t.Data,
		SenderPubKey:  t.SenderPubKey,
		RecipientAddr: t.RecipientAddr,
		Amount:        t.Amount,
		Nonce:        t.Nonce,
	}

	// Marshal to get deterministic bytes
	txBytes, err := proto.Marshal(txToSign)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction for signing: %w", err)
	}

	// Hash the transaction bytes
	txHash := sha256.Sum256(txBytes)

	// Import private key and generate public key
	privKey, pubKey := btcec.PrivKeyFromBytes(privateKey)
	t.SenderPubKey = pubKey.SerializeCompressed()

	// Sign the hash
	signature := ecdsa.Sign(privKey, txHash[:])
	
	// Store the signature
	t.Signature = signature.Serialize()
	// Invalidate cached hash since we modified the transaction
	t.hash = nil
	
	return nil
}

// Verify verifies the transaction's signature.
func (t *Transaction) Verify() error {
	if len(t.Signature) == 0 {
		return fmt.Errorf("transaction is not signed")
	}

	// Recreate the message that was signed
	txToVerify := &pb.Transaction{
		Data:          t.Data,
		SenderPubKey:  t.SenderPubKey,
		RecipientAddr: t.RecipientAddr,
		Amount:        t.Amount,
		Nonce:        t.Nonce,
	}

	// Marshal to get deterministic bytes
	txBytes, err := proto.Marshal(txToVerify)
	if err != nil {
		return fmt.Errorf("failed to marshal transaction for verification: %w", err)
	}

	// Hash the transaction bytes
	txHash := sha256.Sum256(txBytes)

	// Parse the public key
	pubKey, err := btcec.ParsePubKey(t.SenderPubKey)
	if err != nil {
		return fmt.Errorf("invalid sender public key: %w", err)
	}

	// Parse the signature
	sig, err := ecdsa.ParseSignature(t.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Verify the signature
	if !sig.Verify(txHash[:], pubKey) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// Hash calculates and returns the SHA256 hash of the transaction.
// Uses Protobuf marshalling for a deterministic representation.
func (t *Transaction) Hash() ([]byte, error) {
	if t.hash != nil {
		return t.hash, nil
	}

	// Create the corresponding Protobuf message with all fields
	txProto := &pb.Transaction{
		Data:          t.Data,
		SenderPubKey:  t.SenderPubKey,
		RecipientAddr: t.RecipientAddr,
		Amount:        t.Amount,
		Nonce:        t.Nonce,
		Signature:    t.Signature,
	}

	txBytes, err := proto.Marshal(txProto)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transaction: %w", err)
	}

	hash := sha256.Sum256(txBytes)
	t.hash = hash[:] // Cache the hash
	return t.hash, nil
}

// ToProto converts a core.Transaction to its Protobuf representation.
func (t *Transaction) ToProto() (*pb.Transaction, error) {
	hash, err := t.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to get tx hash for proto conversion: %w", err)
	}

	return &pb.Transaction{
		Data:          t.Data,
		SenderPubKey:  t.SenderPubKey,
		RecipientAddr: t.RecipientAddr,
		Amount:        t.Amount,
		Nonce:        t.Nonce,
		Signature:    t.Signature,
		Hash:         hash,
	}, nil
}