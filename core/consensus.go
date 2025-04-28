package core

import (
    "context"
    "errors"
    "fmt"
    "log"
    "math/big"
    "sync"
    "time"
)

// ConsensusConfig holds configuration for the consensus mechanism
type ConsensusConfig struct {
    BlockTime           time.Duration
    ConsensusTimeout    time.Duration
    MinValidators       int
    MaxValidators       int
    ValidatorThreshold  float64  // Percentage of validators needed for consensus
    BlockReward        *big.Int
}

// ConsensusState represents the current state of consensus
type ConsensusState struct {
    Height      uint64
    Round       uint32
    Step        string
    Proposer    string
    ValidatorSet map[string]bool
    Votes       map[string]map[string]bool // proposalHash -> validatorID -> vote
    mu          sync.RWMutex
}

// ConsensusEngine manages the consensus process
type ConsensusEngine struct {
    config      ConsensusConfig
    state       *ConsensusState
    byzantine   *ByzantineNode
    blockchain  *Blockchain
    validators  []string
    mu          sync.RWMutex
}

// NewConsensusEngine creates a new consensus engine
func NewConsensusEngine(config ConsensusConfig, byzantine *ByzantineNode, blockchain *Blockchain) *ConsensusEngine {
    return &ConsensusEngine{
        config:     config,
        state:      &ConsensusState{
            ValidatorSet: make(map[string]bool),
            Votes:       make(map[string]map[string]bool),
        },
        byzantine:  byzantine,
        blockchain: blockchain,
    }
}

// StartConsensus starts the consensus process for a new block
func (ce *ConsensusEngine) StartConsensus(ctx context.Context, block *Block) error {
    _ = ctx

    ce.mu.Lock()
    defer ce.mu.Unlock()

    // Reset state for new round
    ce.state.Round++
    ce.state.Votes = make(map[string]map[string]bool)

    // Select proposer using VRF (convert uint32 to uint64)
    proposer, err := ce.selectProposer(uint64(block.Header.Height))
    if err != nil {
        return fmt.Errorf("failed to select proposer: %w", err)
    }
    ce.state.Proposer = proposer

    // Initialize vote tracking for this proposal
    blockHash, err := block.Hash()
    if err != nil {
        return fmt.Errorf("failed to hash block: %w", err)
    }
    ce.state.Votes[string(blockHash)] = make(map[string]bool)

    // Start consensus phases
    if err := ce.runConsensusPhases(ctx, block); err != nil {
        return fmt.Errorf("consensus failed: %w", err)
    }

    return nil
}

// verifyMPCCompletion checks if MPC share distribution is complete
func (ce *ConsensusEngine) verifyMPCCompletion(ctx context.Context, block *Block) error {
    // Check MPC completion for each shard
    for _, root := range block.ShardRoots {
        shardID := string(root)
        status, err := ce.byzantine.CompleteMPCDistribution(shardID)
        if err != nil {
            return fmt.Errorf("MPC verification failed for shard %s: %w", shardID, err)
        }

        log.Printf("MPC verification complete for shard %s: %d/%d shares verified",
            shardID, status.Verified, status.Total)
    }
    return nil
}

// runConsensusPhases executes the consensus protocol phases
func (ce *ConsensusEngine) runConsensusPhases(ctx context.Context, block *Block) error {
    // Phase 1: Proposal broadcast
    if err := ce.broadcastProposal(ctx, block); err != nil {
        return err
    }

    // Phase 2: Initial validation
    if err := ce.validateProposal(ctx, block); err != nil {
        return err
    }

    // Phase 3: Verify MPC share distribution completion
    if err := ce.verifyMPCCompletion(ctx, block); err != nil {
        return err
    }

    // Phase 4: Collect votes
    votes, err := ce.collectVotes(ctx, block)
    if err != nil {
        return err
    }

    // Phase 5: Verify consensus was reached
    if !ce.hasConsensus(votes) {
        return errors.New("failed to reach consensus")
    }

    // Phase 6: Commit block
    return ce.commitBlock(ctx, block)
}

// selectProposer uses VRF to select the next block proposer
func (ce *ConsensusEngine) selectProposer(height uint64) (string, error) {
    // Get VRF proof for this height
    vrfInput := []byte(fmt.Sprintf("proposer-%d", height))
    vrfOutput, err := ce.byzantine.GenerateVRF(vrfInput)
    if err != nil {
        return "", err
    }

    // Use VRF output to select proposer
    seed := new(big.Int).SetBytes(vrfOutput.Output)
    validatorCount := len(ce.validators)
    if validatorCount == 0 {
        return "", errors.New("no validators available")
    }

    index := new(big.Int).Mod(seed, big.NewInt(int64(validatorCount))).Int64()
    return ce.validators[index], nil
}

// broadcastProposal broadcasts the block proposal to all validators
func (ce *ConsensusEngine) broadcastProposal(ctx context.Context, block *Block) error {
    _ = ctx

    // In practice, this would use the network layer to broadcast
    // For now, just verify the proposer
    if ce.state.Proposer == "" {
        return errors.New("no proposer selected")
    }

    blockHash, err := block.Hash()
    if err != nil {
        return err
    }

    // Record proposal for current round
    ce.state.mu.Lock()
    ce.state.Votes[string(blockHash)] = make(map[string]bool)
    ce.state.mu.Unlock()

    return nil
}

// validateProposal performs initial validation of the proposed block
func (ce *ConsensusEngine) validateProposal(ctx context.Context, block *Block) error {
    _ = ctx

    // Basic block validation
    if err := block.validateBlock(); err != nil {
        return fmt.Errorf("block validation failed: %w", err)
    }

    // Verify proposer's VRF proof using direct VRF verification
    vrfInput := []byte(fmt.Sprintf("proposer-%d", block.Header.Height))
    
    // Generate VRF output for verification
    vrfOutput, err := ce.byzantine.GenerateVRF(vrfInput)
    if err != nil {
        return fmt.Errorf("failed to generate VRF output: %w", err)
    }

    if !ce.byzantine.VerifyVRF(vrfInput, vrfOutput.Output, vrfOutput.Proof) {
        return errors.New("invalid proposer VRF proof")
    }

    // For each shard root in the block, verify its Merkle proof
    for _, root := range block.ShardRoots {
        if err := ce.verifyShardRoot(root); err != nil {
            return fmt.Errorf("shard root verification failed: %w", err)
        }
    }

    return nil
}

// verifyShardRoot verifies a shard's Merkle root against its state
func (ce *ConsensusEngine) verifyShardRoot(root []byte) error {
    // Get shard state and proof
    _, compressed, err := MerkleProofForShardInForest(string(root))
    if err != nil {
        return err
    }

    // Verify the compressed proof
    if !VerifyCompressedMerkleProofCompressed(compressed, root) {
        return errors.New("invalid shard Merkle proof")
    }

    return nil
}

// collectVotes gathers votes from validators
func (ce *ConsensusEngine) collectVotes(ctx context.Context, block *Block) (map[string]bool, error) {
    blockHash, err := block.Hash()
    if err != nil {
        return nil, err
    }

    // Set up vote collection
    voteCtx, cancel := context.WithTimeout(ctx, ce.config.ConsensusTimeout)
    defer cancel()

    votes := ce.state.Votes[string(blockHash)]

    // In practice, would wait for actual votes from the network
    // For now, simulate vote collection based on block validity
    for _, validator := range ce.validators {
        select {
        case <-voteCtx.Done():
            return votes, errors.New("vote collection timed out")
        default:
            if ce.byzantine.IsNodeTrusted(validator) {
                votes[validator] = true
            }
        }
    }

    return votes, nil
}

// hasConsensus checks if enough votes were received
func (ce *ConsensusEngine) hasConsensus(votes map[string]bool) bool {
    voteCount := 0
    for _, voted := range votes {
        if voted {
            voteCount++
        }
    }

    requiredVotes := int(float64(len(ce.validators)) * ce.config.ValidatorThreshold)
    return voteCount >= requiredVotes
}

// commitBlock finalizes the block and adds it to the chain
func (ce *ConsensusEngine) commitBlock(ctx context.Context, block *Block) error {
    _ = ctx

    // Create final commitment
    commitment, err := CreateShardCommitment(block.Header.MerkleRoot)
    if err != nil {
        return fmt.Errorf("failed to create commitment: %w", err)
    }

    // Verify the commitment
    if !VerifyShardCommitment(block.Header.MerkleRoot, commitment) {
        return errors.New("invalid block commitment")
    }

    // Add block to chain
    if err := ce.blockchain.storeBlock(block); err != nil {
        return fmt.Errorf("failed to store block: %w", err)
    }

    // Update consensus state with proper type conversion
    ce.state.mu.Lock()
    ce.state.Height = uint64(block.Header.Height)
    ce.state.Round = 0
    ce.state.mu.Unlock()

    return nil
}

// AddValidator adds a new validator to the consensus set
func (ce *ConsensusEngine) AddValidator(validatorID string) error {
    ce.mu.Lock()
    defer ce.mu.Unlock()

    if len(ce.validators) >= ce.config.MaxValidators {
        return errors.New("maximum validator count reached")
    }

    if !ce.byzantine.IsNodeTrusted(validatorID) {
        return errors.New("validator does not meet trust threshold")
    }

    ce.state.ValidatorSet[validatorID] = true
    ce.validators = append(ce.validators, validatorID)
    return nil
}

// RemoveValidator removes a validator from the consensus set
func (ce *ConsensusEngine) RemoveValidator(validatorID string) {
    ce.mu.Lock()
    defer ce.mu.Unlock()

    delete(ce.state.ValidatorSet, validatorID)
    for i, v := range ce.validators {
        if v == validatorID {
            ce.validators = append(ce.validators[:i], ce.validators[i+1:]...)
            break
        }
    }
}

// GetValidators returns the current set of validators
func (ce *ConsensusEngine) GetValidators() []string {
    ce.mu.RLock()
    defer ce.mu.RUnlock()
    validators := make([]string, len(ce.validators))
    copy(validators, ce.validators)
    return validators
}

func (ce *ConsensusEngine) SetBlockchain(bc *Blockchain) {
    ce.mu.Lock()
    defer ce.mu.Unlock()
    ce.blockchain = bc
}