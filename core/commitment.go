package core

import (
	"crypto/rand"
	"math/big"
	"github.com/btcsuite/btcd/btcec/v2"
)

// PedersenCommitment represents a Pedersen commitment (C = v*G + r*H)
type PedersenCommitment struct {
	Commitment *btcec.PublicKey
	Blinding   *big.Int
	Value      *big.Int
}

// GeneratePedersenCommitment creates a Pedersen commitment to a value using a random blinding factor.
func GeneratePedersenCommitment(value *big.Int) (*PedersenCommitment, error) {
	curve := btcec.S256()
	G := curve.Params().Gx
	Gy := curve.Params().Gy

	// Use a fixed H = G*2 for demo (not secure for production)
	Hx, Hy := curve.ScalarMult(G, Gy, big.NewInt(2).Bytes())

	r, err := rand.Int(rand.Reader, curve.Params().N)
	if err != nil {
		return nil, err
	}

	// v*G
	vGx, vGy := curve.ScalarMult(G, Gy, value.Bytes())
	// r*H
	rHx, rHy := curve.ScalarMult(Hx, Hy, r.Bytes())
	// C = v*G + r*H
	Cx, Cy := curve.Add(vGx, vGy, rHx, rHy)

	// Convert coordinates to compressed point format
	isOdd := Cy.Bit(0) == 1
	compressedPoint := make([]byte, 33)
	if isOdd {
		compressedPoint[0] = 0x03
	} else {
		compressedPoint[0] = 0x02
	}
	Cx.FillBytes(compressedPoint[1:])

	// Parse the compressed point to create a public key
	commitment, err := btcec.ParsePubKey(compressedPoint)
	if err != nil {
		return nil, err
	}

	return &PedersenCommitment{
		Commitment: commitment,
		Blinding:   r,
		Value:      value,
	}, nil
}

// VerifyPedersenCommitment checks if the commitment matches the value and blinding factor.
func VerifyPedersenCommitment(commitment *PedersenCommitment) bool {
	curve := btcec.S256()
	G := curve.Params().Gx
	Gy := curve.Params().Gy
	Hx, Hy := curve.ScalarMult(G, Gy, big.NewInt(2).Bytes())
	vGx, vGy := curve.ScalarMult(G, Gy, commitment.Value.Bytes())
	rHx, rHy := curve.ScalarMult(Hx, Hy, commitment.Blinding.Bytes())
	Cx, Cy := curve.Add(vGx, vGy, rHx, rHy)
	return Cx.Cmp(commitment.Commitment.X()) == 0 && Cy.Cmp(commitment.Commitment.Y()) == 0
}
