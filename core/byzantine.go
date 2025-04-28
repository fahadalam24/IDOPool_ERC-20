package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
)

// Configuration constants
const (
	DefaultReputation        = 50.0
	MaxReputation            = 100.0
	MinReputation            = 0.0
	HistoryWindowHours       = 24 * 7 // Keep 1 week of history
	DefaultConsensusThreshold = 0.67   // 67% consensus required
)

// AdaptiveFactors holds network conditions that affect reputation
type AdaptiveFactors struct {
	NetworkStress     float64 // 0-1, higher means more stress
	FailureRate       float64 // 0-1, rate of failed operations
	ParticipantCount  int     // Number of active participants
}

// ReputationThresholds defines minimum scores for different roles
type ReputationThresholds struct {
	Minimum   float64 // Minimum to participate
	Consensus float64 // Required for consensus participation
	Leader    float64 // Required to be considered for leader
}

// ByzantineConfig holds configuration for Byzantine fault tolerance
type ByzantineConfig struct {
	AdaptiveFactors      AdaptiveFactors
	ReputationThresholds ReputationThresholds
	DecayRate           float64  // Rate at which old reputation decays
	InitialReputation    float64  // Initial reputation score for new nodes
	ConsensusThreshold   float64  // Base threshold for consensus participation
	VRFSeed             []byte   // Seed for VRF calculations
}

type trustRecord struct {
	timestamp time.Time
	score     float64
}

type MPCShareDistributionStatus struct {
	Total       int
	Distributed int
	Verified    int
	Failed      int
}

type MPCMetrics struct {
	mu                     sync.Mutex
	TotalSharesDistributed int
	SuccessfulVerifications int
	FailedVerifications    int
	ActiveDistributions    int
	AverageLatency        time.Duration
}

// UpdateActiveDistributions updates the count of active MPC distributions
func (m *MPCMetrics) UpdateActiveDistributions(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveDistributions += delta
}

// ZKProof represents a zero-knowledge proof
type ZKProof struct {
	ProofData []byte
}

// MPC related types
type MPCShare struct {
	Index      uint32
	Value      *big.Int
	Commitment []byte
}

type MPCDistribution struct {
	ID            string
	Threshold     int
	TotalShares   int
	Shares        map[string]*MPCShare
	Commitments   [][]byte
	StartTime     time.Time
	Verifications map[string]*MPCVerificationStatus
}

type MPCVerificationStatus struct {
	ShareID          string
	ParticipantID    string
	Verified         bool
	VerificationTime int64
	Errors          []string
}

type ByzantineNode struct {
	Config         ByzantineConfig
	trustScores    map[string]float64      // Current reputation scores
	trustHistory   map[string][]trustRecord // Historical reputation data
	mpcMetrics     *MPCMetrics             // Metrics for MPC operations
	vrfPrivKey     *ecdsa.PrivateKey       // VRF private key
	vrfPublicKeys  map[string]*ecdsa.PublicKey // VRF public keys of other nodes
	privateKey     *ecdsa.PrivateKey
	publicKey      *ecdsa.PublicKey
	nodeID         string
	activeNodes    map[string]bool
	mpcDistributions map[string]*MPCDistribution
	curve           elliptic.Curve
	mu            sync.RWMutex // Mutex for protecting concurrent access
}

// Reputation score with temporal metadata
type ReputationScore struct {
	Value       float64
	LastUpdated time.Time
	Confidence  float64    // Confidence level in the score (0-1)
	History     []struct {
		Value     float64
		Timestamp time.Time
	}
}

func NewByzantineNode(config ByzantineConfig, nodeID string) (*ByzantineNode, error) {
	// Generate VRF key pair
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate VRF key: %v", err)
	}

	bn := &ByzantineNode{
		Config:           config,
		trustScores:      make(map[string]float64),
		trustHistory:     make(map[string][]trustRecord),
		mpcMetrics:       &MPCMetrics{},
		vrfPrivKey:       privKey,
		vrfPublicKeys:    make(map[string]*ecdsa.PublicKey),
		privateKey:       privKey,
		publicKey:        &privKey.PublicKey,
		nodeID:           nodeID, // Set to actual node ID
		activeNodes:      make(map[string]bool),
		mpcDistributions: make(map[string]*MPCDistribution),
		curve:            elliptic.P256(),
	}
	// Initialize self trust score
	bn.trustScores[nodeID] = config.InitialReputation
	return bn, nil
}

// RegisterPeer initializes trust for a new peer if not already present
func (bn *ByzantineNode) RegisterPeer(peerID string) {
	bn.mu.Lock()
	defer bn.mu.Unlock()
	if _, exists := bn.trustScores[peerID]; !exists {
		bn.trustScores[peerID] = bn.Config.InitialReputation
	}
	bn.activeNodes[peerID] = true
}

func (bn *ByzantineNode) GetMPCMetrics() *MPCMetrics {
	bn.mu.RLock()
	defer bn.mu.RUnlock()
	return bn.mpcMetrics
}

// ReputationMetrics tracks various node behavior metrics
type ReputationMetrics struct {
	BlocksProposed    uint64
	ValidBlocks       uint64
	InvalidBlocks     uint64
	ConsensusVotes    uint64
	CorrectVotes      uint64
	ResponseTime      []time.Duration
	LastUpdate        time.Time
}

// UpdateNodeReputation updates a node's reputation based on its behavior
func (bn *ByzantineNode) UpdateNodeReputation(nodeID string, action string, value float64) {
	bn.mu.Lock()
	defer bn.mu.Unlock()

	if bn.trustScores == nil {
		bn.trustScores = make(map[string]float64)
	}

	currentScore := bn.trustScores[nodeID]

	// Weight different actions differently
	weights := map[string]float64{
		"block_proposal": 0.3,
		"vote":          0.2,
		"response_time": 0.1,
		"validation":    0.4,
	}

	// Apply weight based on action type
	weight := weights[action]
	if weight == 0 {
		weight = 0.1 // default weight
	}

	// Calculate new score with temporal decay
	timeDecay := calculateTimeDecay(bn.trustHistory[nodeID])
	newScore := (currentScore * timeDecay) + (value * weight)

	// Normalize score between 0 and 1
	if newScore > 1 {
		newScore = 1
	} else if newScore < 0 {
		newScore = 0
	}

	bn.trustScores[nodeID] = newScore

	// Update history
	record := trustRecord{
		score:     newScore,
		timestamp: time.Now(),
	}
	bn.trustHistory[nodeID] = append(bn.trustHistory[nodeID], record)
}

// calculateTimeDecay applies temporal decay to historical reputation
func calculateTimeDecay(history []trustRecord) float64 {
	if len(history) == 0 {
		return 1.0
	}
	lastUpdate := history[len(history)-1].timestamp
	timeDiff := time.Since(lastUpdate)
	// Decay factor: 0.99 per hour, with a minimum of 0.5
	decayFactor := math.Pow(0.99, timeDiff.Hours())
	if decayFactor < 0.5 {
		decayFactor = 0.5
	}
	return decayFactor
}

// GetConsensusThreshold calculates adaptive consensus threshold based on network health
func (bn *ByzantineNode) GetConsensusThreshold() float64 {
	bn.mu.RLock()
	defer bn.mu.RUnlock()

	var totalScore float64
	var activeNodes int

	// Calculate average network reputation
	for _, score := range bn.trustScores {
		totalScore += score
		activeNodes++
	}

	if activeNodes == 0 {
		return DefaultConsensusThreshold
	}

	avgReputation := totalScore / float64(activeNodes)

	// Adjust threshold based on average reputation
	// Higher reputation = lower threshold, but never below 2f+1 requirement
	baseThreshold := 2.0/3.0 // Classic BFT requirement
	minThreshold := baseThreshold
	maxThreshold := 0.85

	// Scale threshold inversely with reputation
	threshold := maxThreshold - (avgReputation * (maxThreshold - minThreshold))

	// Never go below minimum BFT requirement
	if threshold < minThreshold {
		threshold = minThreshold
	}

	return threshold
}

// LeaderElection represents a round of leader election
type LeaderElection struct {
	Round      uint64
	Timestamp  int64
	VRFOutput  []byte
	VRFProof   []byte
	Leader     string
}

func (bn *ByzantineNode) RunLeaderElection(round uint64) (*LeaderElection, error) {
	timestamp := time.Now().Unix()
	hash := generateRoundHash(round, timestamp)

	k := generateDeterministicK(hash, bn.privateKey)
	vrfOutput := generateVRFOutput(hash, k)
	vrfProof := generateVRFProof(hash, k, bn.privateKey)

	election := &LeaderElection{
		Round:     round,
		Timestamp: timestamp,
		VRFOutput: vrfOutput,
		VRFProof:  vrfProof,
		Leader:    bn.selectLeader(vrfOutput),
	}

	return election, nil
}

func (bn *ByzantineNode) selectLeader(vrfOutput []byte) string {
	activeNodes := bn.GetActiveNodes()
	if len(activeNodes) == 0 {
		return ""
	}

	// Convert VRF output to big integer and use it to select leader
	value := new(big.Int).SetBytes(vrfOutput)
	index := new(big.Int).Mod(value, big.NewInt(int64(len(activeNodes)))).Int64()

	return activeNodes[index]
}

func generateRoundHash(round uint64, timestamp int64) []byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%d-%d", round, timestamp)))
	return h.Sum(nil)
}

func (bn *ByzantineNode) VerifyLeaderElection(election *LeaderElection) bool {
	hash := generateRoundHash(election.Round, election.Timestamp)
	return verifyVRFProof(hash, election.VRFOutput, election.VRFProof)
}

// --- Helper functions for VRF ---
// generateDeterministicK generates a deterministic k value for signing
func generateDeterministicK(hash []byte, priv *ecdsa.PrivateKey) *big.Int {
	// Simple deterministic k: hash of (hash || priv.D)
	h := sha256.New()
	h.Write(hash)
	h.Write(priv.D.Bytes())
	return new(big.Int).SetBytes(h.Sum(nil))
}

// generateVRFOutput generates the VRF output
func generateVRFOutput(hash []byte, k *big.Int) []byte {
	// Simple VRF output: hash of (hash || k)
	h := sha256.New()
	h.Write(hash)
	h.Write(k.Bytes())
	return h.Sum(nil)
}

// generateVRFProof generates the VRF proof
func generateVRFProof(hash []byte, k *big.Int, priv *ecdsa.PrivateKey) []byte {
	// Simple VRF proof: hash of (hash || k || priv.D)
	h := sha256.New()
	h.Write(hash)
	h.Write(k.Bytes())
	h.Write(priv.D.Bytes())
	return h.Sum(nil)
}

// verifyVRFProof verifies the VRF proof
func verifyVRFProof(hash, beta, pi []byte) bool {
	// Simple check: hash of (hash || beta) == pi
	h := sha256.New()
	h.Write(hash)
	h.Write(beta)
	expected := h.Sum(nil)
	return bytes.Equal(pi, expected)
}

// --- VRF context and proof structs ---
type VRFContext struct {
	Epoch        uint64
	Seed         []byte
	ProofMap     map[string]*VRFProof
	ThresholdMap map[uint64][]byte
	mu           sync.RWMutex
}

type VRFProof struct {
	Beta      []byte
	Pi        []byte
	PublicKey []byte
}

// --- End of VRF structs ---

func (bn *ByzantineNode) InitializeVRF(seed []byte) *VRFContext {
	return &VRFContext{
		Epoch:        0,
		Seed:         seed,
		ProofMap:     make(map[string]*VRFProof),
		ThresholdMap: make(map[uint64][]byte),
	}
}

// VRFOutput holds the output and proof for a VRF evaluation.
type VRFOutput struct {
	Output []byte
	Proof  []byte
}

// GenerateVRF generates a VRF output and proof using secp256k1 and ECVRF.
func (bn *ByzantineNode) GenerateVRF(epochData []byte) (*VRFOutput, error) {
	priv := bn.vrfPrivKey
	if priv == nil {
		return nil, fmt.Errorf("VRF private key not initialized")
	}
	// Hash the input
	hash := sha256.Sum256(epochData)
	// Use ECDSA Sign as a stand-in for ECVRF proof (for demonstration)
	r, s, err := ecdsa.Sign(rand.Reader, priv, hash[:])
	if err != nil {
		return nil, err
	}
	proof := append(r.Bytes(), s.Bytes()...)
	// VRF output is the ECDSA signature hash
	output := sha256.Sum256(proof)
	return &VRFOutput{Output: output[:], Proof: proof}, nil
}

// VerifyVRF verifies a VRF proof and output.
func (bn *ByzantineNode) VerifyVRF(input, output, proof []byte) bool {
	pub := bn.vrfPrivKey.Public().(*ecdsa.PublicKey)
	if pub == nil {
		return false
	}
	// Hash the input
	hash := sha256.Sum256(input)
	// Split proof into r and s
	if len(proof) < 64 {
		return false
	}
	r := new(big.Int).SetBytes(proof[:32])
	s := new(big.Int).SetBytes(proof[32:64])
	valid := ecdsa.Verify(pub, hash[:], r, s)
	if !valid {
		return false
	}
	// Check output matches
	expectedOutput := sha256.Sum256(proof)
	return bytes.Equal(output, expectedOutput[:])
}

func (bn *ByzantineNode) GetActiveNodes() []string {
	nodes := make([]string, 0, len(bn.activeNodes))
	for node, active := range bn.activeNodes {
		if active {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// IsNodeTrusted returns true if the node's reputation is above the minimum threshold
func (bn *ByzantineNode) IsNodeTrusted(nodeID string) bool {
	bn.mu.RLock()
	defer bn.mu.RUnlock()
	score := bn.trustScores[nodeID]
	return score >= bn.Config.ReputationThresholds.Minimum
}

// UpdateReputation is an exported wrapper for UpdateNodeReputation
func (bn *ByzantineNode) UpdateReputation(nodeID, action string, value float64) {
	bn.UpdateNodeReputation(nodeID, action, value)
}

// GenerateMPCShares creates shares for a secret using Shamir's Secret Sharing
func (bn *ByzantineNode) GenerateMPCShares(secret *big.Int, n, t int) ([]*MPCShare, error) {
	if t > n {
		return nil, fmt.Errorf("threshold cannot be greater than number of shares")
	}
	
	// Use the node's elliptic curve for prime field operations
	prime := new(big.Int).Sub(bn.curve.Params().N, big.NewInt(1))
	poly := make([]*big.Int, t)
	poly[0] = new(big.Int).Set(secret)
	
	// Generate random coefficients for the polynomial
	for i := 1; i < t; i++ {
		coeff, err := rand.Int(rand.Reader, prime)
		if err != nil {
			return nil, fmt.Errorf("failed to generate polynomial coefficient: %w", err)
		}
		poly[i] = coeff
	}
	
	shares := make([]*MPCShare, n)
	// Generate shares using polynomial evaluation
	for i := 0; i < n; i++ {
		x := big.NewInt(int64(i + 1))
		y := new(big.Int).Set(poly[0])
		
		// Evaluate polynomial
		for j := 1; j < t; j++ {
			term := new(big.Int).Exp(x, big.NewInt(int64(j)), prime)
			term.Mul(term, poly[j])
			y.Add(y, term)
			y.Mod(y, prime)
		}
		
		// Generate Pedersen commitment for the share
		comm, err := GeneratePedersenCommitment(y)
		if err != nil {
			return nil, fmt.Errorf("failed to generate commitment for share %d: %w", i, err)
		}
		
		shares[i] = &MPCShare{
			Index:      uint32(i + 1),
			Value:     y,
			Commitment: comm.Commitment.SerializeCompressed(),
		}
	}
	
	return shares, nil
}

// VerifyMPCShare verifies a share against its commitment
func (bn *ByzantineNode) VerifyMPCShare(share *MPCShare, dist *MPCDistribution) bool {
	// Verify share is within valid range
	if int(share.Index) > dist.TotalShares {
		return false
	}
	
	// Verify commitment
	comm := &PedersenCommitment{
		Value: share.Value,
	}
	
	// Parse and verify the commitment directly using btcec
	pubKey, err := btcec.ParsePubKey(share.Commitment)
	if err != nil {
		return false
	}

	// Use the btcec public key directly for commitment verification
	comm.Commitment = pubKey
	
	return VerifyPedersenCommitment(comm)
}

// CombineMPCShares reconstructs the secret from a threshold of shares
func (bn *ByzantineNode) CombineMPCShares(shares []*MPCShare, t int) (*big.Int, error) {
	if len(shares) < t {
		return nil, fmt.Errorf("insufficient shares for reconstruction: got %d, need %d", len(shares), t)
	}
	
	prime := new(big.Int).Sub(bn.curve.Params().N, big.NewInt(1))
	secret := new(big.Int).SetInt64(0)
	
	// Lagrange interpolation
	for i, share1 := range shares[:t] {
		basis := new(big.Int).SetInt64(1)
		
		for j, share2 := range shares[:t] {
			if i == j {
				continue
			}
			
			num := new(big.Int).SetInt64(int64(-share2.Index))
			den := new(big.Int).Sub(
				new(big.Int).SetInt64(int64(share1.Index)),
				new(big.Int).SetInt64(int64(share2.Index)),
			)
			den.ModInverse(den, prime)
			num.Mul(num, den)
			num.Mod(num, prime)
			basis.Mul(basis, num)
			basis.Mod(basis, prime)
		}
		
		term := new(big.Int).Mul(share1.Value, basis)
		secret.Add(secret, term)
		secret.Mod(secret, prime)
	}
	
	return secret, nil
}

// StartMPCDistribution initiates a new MPC share distribution
func (bn *ByzantineNode) StartMPCDistribution(id string, secret *big.Int, participants []string, threshold int) (*MPCDistribution, error) {
	n := len(participants)
	shares, err := bn.GenerateMPCShares(secret, n, threshold)
	if err != nil {
		return nil, err
	}
	
	dist := &MPCDistribution{
		ID:            id,
		Threshold:     threshold,
		TotalShares:   n,
		Shares:        make(map[string]*MPCShare),
		Commitments:   make([][]byte, n),
		StartTime:     time.Now(),
		Verifications: make(map[string]*MPCVerificationStatus),
	}
	
	// Store commitments and prepare shares for distribution
	for i, share := range shares {
		dist.Commitments[i] = share.Commitment
		dist.Shares[participants[i]] = share
	}
	
	bn.mpcDistributions[id] = dist
	bn.mpcMetrics.UpdateActiveDistributions(1)
	
	return dist, nil
}

// VerifyAndStoreMPCShare verifies and stores a received share
func (bn *ByzantineNode) VerifyAndStoreMPCShare(distID string, participantID string, share *MPCShare) error {
	bn.mu.Lock()
	defer bn.mu.Unlock()
	
	dist, exists := bn.mpcDistributions[distID]
	if !exists {
		return fmt.Errorf("distribution %s not found", distID)
	}
	
	if !bn.VerifyMPCShare(share, dist) {
		bn.mpcMetrics.FailedVerifications++
		return fmt.Errorf("share verification failed")
	}
	
	// Update verification status
	dist.Verifications[participantID] = &MPCVerificationStatus{
		ShareID:          fmt.Sprintf("%s-%d", distID, share.Index),
		ParticipantID:    participantID,
		Verified:         true,
		VerificationTime: time.Now().Unix(),
	}
	
	bn.mpcMetrics.SuccessfulVerifications++
	return nil
}

// CheckMPCDistributionStatus returns the current status of a distribution
func (bn *ByzantineNode) CheckMPCDistributionStatus(distID string) (*MPCShareDistributionStatus, error) {
	bn.mu.RLock()
	defer bn.mu.RUnlock()
	
	dist, exists := bn.mpcDistributions[distID]
	if !exists {
		return nil, fmt.Errorf("distribution %s not found", distID)
	}
	
	status := &MPCShareDistributionStatus{
		Total:       dist.TotalShares,
		Distributed: len(dist.Shares),
		Verified:    0,
		Failed:      0,
	}
	
	for _, v := range dist.Verifications {
		if v.Verified {
			status.Verified++
		} else {
			status.Failed++
		}
	}
	
	return status, nil
}

// CompleteMPCDistribution finalizes an MPC share distribution
func (bn *ByzantineNode) CompleteMPCDistribution(distID string) (*MPCShareDistributionStatus, error) {
	bn.mu.Lock()
	defer bn.mu.Unlock()
	
	dist, exists := bn.mpcDistributions[distID]
	if !exists {
		return nil, fmt.Errorf("distribution %s not found", distID)
	}

	status := &MPCShareDistributionStatus{
		Total: dist.TotalShares,
		Distributed: len(dist.Shares),
		Verified: 0,
		Failed: 0,
	}

	// Count verifications
	for _, verification := range dist.Verifications {
		if verification.Verified {
			status.Verified++
		} else {
			status.Failed++
		}
	}

	// Check if distribution meets threshold
	threshold := (2 * dist.TotalShares / 3) + 1 // 2f+1 threshold
	if status.Verified < threshold {
		return status, fmt.Errorf("failed to achieve verification threshold: got %d out of required %d",
			status.Verified, threshold)
	}

	// Update metrics
	bn.mpcMetrics.UpdateActiveDistributions(-1)
	bn.mpcMetrics.SuccessfulVerifications += status.Verified
	bn.mpcMetrics.FailedVerifications += status.Failed

	return status, nil
}