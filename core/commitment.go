package core

import (
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
)

// PedersenCommitment represents a Pedersen commitment (C = v*G + r*H)
type PedersenCommitment struct {
	Commitment *btcec.PublicKey
	Blinding   *big.Int
	Value      *big.Int
}

var (
	pedersenH   *btcec.PublicKey
	initPedersen sync.Once
)

// hashToCurve deterministically hashes a string to a curve point (for H)
func hashToCurve(label string) *btcec.PublicKey {
	curve := btcec.S256()
	counter := 0
	for {
		input := append([]byte(label), byte(counter))
		hash := sha256.Sum256(input)
		x, y := curve.ScalarBaseMult(hash[:])
		if y != nil {
			return btcec.NewPublicKey(bigIntToFieldVal(x), bigIntToFieldVal(y))
		}
		counter++
	}
}

// getPedersenH returns a securely generated H independent of G
func getPedersenH() *btcec.PublicKey {
	initPedersen.Do(func() {
		pedersenH = hashToCurve("pedersen_commitment_H")
	})
	return pedersenH
}

// bigIntToBytes converts a big.Int to a 32-byte slice
func bigIntToBytes(bi *big.Int) []byte {
	bytes := bi.Bytes()
	if len(bytes) > 32 {
		bytes = bytes[:32]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(bytes):], bytes)
	return padded
}

// Helper to convert *big.Int to *btcec.FieldVal
func bigIntToFieldVal(b *big.Int) *btcec.FieldVal {
	var fv btcec.FieldVal
	fv.SetByteSlice(b.Bytes())
	return &fv
}

// GeneratePedersenCommitment creates a secure Pedersen commitment to a value using a random blinding factor.
func GeneratePedersenCommitment(value *big.Int) (*PedersenCommitment, error) {
	curve := btcec.S256()
	H := getPedersenH()

	// Generate random blinding factor
	r, err := rand.Int(rand.Reader, curve.N)
	if err != nil {
		return nil, err
	}

	// v*G using the generator point
	vBytes := bigIntToBytes(value)
	vPrivKey, _ := btcec.PrivKeyFromBytes(vBytes)
	vG := vPrivKey.PubKey()

	// r*H using the independent point H
	rBytes := bigIntToBytes(r)
	rHx, rHy := curve.ScalarMult(H.X(), H.Y(), rBytes)

	// C = v*G + r*H
	commitmentX, commitmentY := curve.Add(vG.X(), vG.Y(), rHx, rHy)
	commitment := btcec.NewPublicKey(bigIntToFieldVal(commitmentX), bigIntToFieldVal(commitmentY))

	return &PedersenCommitment{
		Commitment: commitment,
		Blinding:   r,
		Value:      value,
	}, nil
}

// VerifyPedersenCommitment checks if the commitment matches the value and blinding factor.
func VerifyPedersenCommitment(commitment *PedersenCommitment) bool {
	curve := btcec.S256()
	H := getPedersenH()

	// v*G
	vBytes := bigIntToBytes(commitment.Value)
	vPrivKey, _ := btcec.PrivKeyFromBytes(vBytes)
	vG := vPrivKey.PubKey()

	// r*H
	rBytes := bigIntToBytes(commitment.Blinding)
	rHx, rHy := curve.ScalarMult(H.X(), H.Y(), rBytes)

	// Expected commitment = v*G + r*H
	expectedX, expectedY := curve.Add(vG.X(), vG.Y(), rHx, rHy)
	expected := btcec.NewPublicKey(bigIntToFieldVal(expectedX), bigIntToFieldVal(expectedY))

	return expected.IsEqual(commitment.Commitment)
}

// OpenCommitment reveals the value and blinding factor of a commitment
func OpenCommitment(c *PedersenCommitment) (value *big.Int, blinding *big.Int) {
	return c.Value, c.Blinding
}

// CreateShardCommitment creates a Pedersen commitment for a shard's state
func CreateShardCommitment(shardState []byte) (*PedersenCommitment, error) {
	// Convert shard state to a numerical value
	stateHash := sha256.Sum256(shardState)
	value := new(big.Int).SetBytes(stateHash[:])

	// Generate commitment
	return GeneratePedersenCommitment(value)
}

// VerifyShardCommitment verifies a shard's state commitment
func VerifyShardCommitment(shardState []byte, commitment *PedersenCommitment) bool {
	// Regenerate state value
	stateHash := sha256.Sum256(shardState)
	expectedValue := new(big.Int).SetBytes(stateHash[:])

	// Check if the committed value matches
	if expectedValue.Cmp(commitment.Value) != 0 {
		return false
	}

	// Verify the commitment itself
	return VerifyPedersenCommitment(commitment)
}

// BatchVerifyCommitments verifies multiple commitments in parallel
func BatchVerifyCommitments(commitments []*PedersenCommitment) bool {
	if len(commitments) == 0 {
		return true
	}

	results := make(chan bool, len(commitments))
	for _, comm := range commitments {
		go func(c *PedersenCommitment) {
			results <- VerifyPedersenCommitment(c)
		}(comm)
	}

	// Collect results
	for i := 0; i < len(commitments); i++ {
		if !<-results {
			return false
		}
	}
	return true
}
