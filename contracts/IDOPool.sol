// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/security/ReentrancyGuard.sol";

/**
 * @title OwnableWithInit
 * @dev Extension of OpenZeppelin's Ownable that allows setting the owner in the constructor.
 * This is necessary because the Ownable constructor in older OpenZeppelin versions doesn't 
 * take an initial owner parameter.
 */
abstract contract OwnableWithInit is Ownable {
    constructor(address initialOwner) {
        _transferOwnership(initialOwner);
    }
}

/**
 * @title IDOPool
 * @dev A contract for Initial DEX Offering (IDO) that accepts payments in ERC-20 tokens
 * and includes refund mechanisms for both users and admins.
 *
 * Key features:
 * - Users can buy tokens using a specific ERC-20 token (not ETH)
 * - Configurable soft cap and hard cap for fundraising
 * - Refund mechanism when soft cap is not met
 * - Admin can trigger global refund in case of emergency
 * - Token claims only enabled after successful IDO
 * - Protection against reentrancy attacks
 * - Proper validation of ERC-20 token addresses
 */
contract IDOPool is OwnableWithInit, ReentrancyGuard {
    using SafeERC20 for IERC20;

    // Token being sold
    IERC20 public idoToken;
    // Token accepted for payment
    IERC20 public paymentToken;
    
    // IDO configuration
    uint256 public startTime;
    uint256 public endTime;
    uint256 public softCap;
    uint256 public hardCap;
    uint256 public tokenPrice; // Price of 1 IDO token in payment token units
    
    // Refund configuration
    uint256 public refundEndTime;
    bool public refundsEnabled;
    bool public globalRefundEnabled;
    
    // Token distribution
    uint256 public totalTokensSold;
    uint256 public totalPaymentCollected;
    bool public tokensClaimable;
    
    // Mapping of user contributions and claims
    mapping(address => uint256) public contributions;
    mapping(address => bool) public hasClaimed;
    mapping(address => bool) public hasRefunded;
    
    // Events
    event TokensPurchased(address indexed buyer, uint256 paymentAmount, uint256 tokenAmount);
    event TokensClaimed(address indexed claimer, uint256 tokenAmount);
    event RefundIssued(address indexed user, uint256 amount);
    event GlobalRefundEnabled();
    event IDOStarted(uint256 startTime, uint256 endTime);
    event IDOEnded();
    event TokensClaimable();

    /**
     * @dev Constructor to initialize the IDO Pool
     * @param _idoToken Address of the token being sold
     * @param _paymentToken Address of the token accepted for payment
     * @param _tokenPrice Price of 1 IDO token in payment token units
     * @param _softCap Minimum amount of payment tokens to raise
     * @param _hardCap Maximum amount of payment tokens to raise
     */    constructor(
        address _idoToken,
        address _paymentToken,
        uint256 _tokenPrice,
        uint256 _softCap,
        uint256 _hardCap
    ) OwnableWithInit(msg.sender) {
        require(_idoToken != address(0), "Invalid IDO token address");
        require(_paymentToken != address(0), "Invalid payment token address");
        require(_tokenPrice > 0, "Token price must be greater than 0");
        require(_softCap > 0, "Soft cap must be greater than 0");
        require(_hardCap >= _softCap, "Hard cap must be greater than or equal to soft cap");
        
        // Validate if provided token addresses implement ERC20 interface
        // This is a basic check, but more thorough validation would require testing token functions
        try IERC20(_idoToken).totalSupply() returns (uint256) {} 
        catch {
            revert("IDO token does not conform to ERC20 standard");
        }
        
        try IERC20(_paymentToken).totalSupply() returns (uint256) {}
        catch {
            revert("Payment token does not conform to ERC20 standard");
        }
        
        idoToken = IERC20(_idoToken);
        paymentToken = IERC20(_paymentToken);
        tokenPrice = _tokenPrice;
        softCap = _softCap;
        hardCap = _hardCap;
        refundsEnabled = false;
        globalRefundEnabled = false;
        tokensClaimable = false;
    }
    
    /**
     * @dev Start the IDO with specified duration
     * @param _startTime Start time of the IDO (unix timestamp)
     * @param _endTime End time of the IDO (unix timestamp)
     */
    function startIDO(uint256 _startTime, uint256 _endTime) external onlyOwner {
        require(_startTime >= block.timestamp, "Start time must be in the future");
        require(_endTime > _startTime, "End time must be after start time");
        require(startTime == 0, "IDO already started");
        
        startTime = _startTime;
        endTime = _endTime;
        
        emit IDOStarted(_startTime, _endTime);
    }
    
    /**
     * @dev End the IDO early
     */
    function endIDO() external onlyOwner {
        require(startTime > 0, "IDO not started");
        require(block.timestamp >= startTime, "IDO has not started yet");
        require(block.timestamp < endTime, "IDO already ended");
        
        endTime = block.timestamp;
        
        emit IDOEnded();
    }
    
    /**
     * @dev Set refund window
     * @param _refundEndTime End time for refund period (unix timestamp)
     */
    function setRefundWindow(uint256 _refundEndTime) external onlyOwner {
        require(_refundEndTime > block.timestamp, "Refund end time must be in the future");
        refundEndTime = _refundEndTime;
    }
    
    /**
     * @dev Enable or disable refunds
     * @param _refundsEnabled Whether refunds are enabled
     */
    function setRefundsEnabled(bool _refundsEnabled) external onlyOwner {
        refundsEnabled = _refundsEnabled;
    }
    
    /**
     * @dev Enable global refund, allowing all participants to withdraw their funds
     */
    function enableGlobalRefund() external onlyOwner {
        require(!tokensClaimable, "Cannot enable global refund after tokens are claimable");
        globalRefundEnabled = true;
        refundsEnabled = true;
        
        emit GlobalRefundEnabled();
    }
    
    /**
     * @dev Allow tokens to be claimed by participants
     */
    function enableTokenClaims() external onlyOwner {
        require(block.timestamp >= endTime, "IDO has not ended yet");
        require(totalPaymentCollected >= softCap, "Soft cap not reached");
        require(!globalRefundEnabled, "Cannot enable token claims when global refund is enabled");
        
        tokensClaimable = true;
        refundsEnabled = false;  // Disable refunds when tokens are claimable
        
        emit TokensClaimable();
    }
    
    /**
     * @dev Buy tokens with the accepted payment token
     * @param amount Amount of payment tokens to spend
     */
    function buyTokens(uint256 amount) external nonReentrant {
        require(block.timestamp >= startTime, "IDO has not started yet");
        require(block.timestamp <= endTime, "IDO has ended");
        require(amount > 0, "Amount must be greater than 0");
        require(totalPaymentCollected + amount <= hardCap, "Exceeds hard cap");
        
        // Calculate tokens to purchase
        uint256 tokensAmount = (amount * (10**18)) / tokenPrice;
        require(tokensAmount > 0, "Token amount too small");
        
        // Transfer payment tokens from user to contract
        paymentToken.safeTransferFrom(msg.sender, address(this), amount);
        
        // Update state
        contributions[msg.sender] += amount;
        totalPaymentCollected += amount;
        totalTokensSold += tokensAmount;
        
        emit TokensPurchased(msg.sender, amount, tokensAmount);
    }
    
    /**
     * @dev Claim purchased tokens
     */
    function claimTokens() external nonReentrant {
        require(tokensClaimable, "Tokens are not claimable yet");
        require(contributions[msg.sender] > 0, "No contribution found");
        require(!hasClaimed[msg.sender], "Already claimed tokens");
        
        uint256 paymentAmount = contributions[msg.sender];
        uint256 tokenAmount = (paymentAmount * (10**18)) / tokenPrice;
        
        hasClaimed[msg.sender] = true;
        
        // Transfer IDO tokens to user
        idoToken.safeTransfer(msg.sender, tokenAmount);
        
        emit TokensClaimed(msg.sender, tokenAmount);
    }
    
    /**
     * @dev Claim refund for contributed payment tokens
     */
    function claimRefund() external nonReentrant {
        require(refundsEnabled, "Refunds are not enabled");
        require(contributions[msg.sender] > 0, "No contribution found");
        require(!hasClaimed[msg.sender], "Cannot refund after claiming tokens");
        require(!hasRefunded[msg.sender], "Already refunded");
        
        // Check additional refund conditions if global refund is not enabled
        if (!globalRefundEnabled) {
            require(block.timestamp <= refundEndTime, "Refund period has ended");
            require(totalPaymentCollected < softCap || block.timestamp > endTime, 
                    "Refund condition not met");
        }
        
        uint256 refundAmount = contributions[msg.sender];
        hasRefunded[msg.sender] = true;
        
        // Transfer payment tokens back to user
        paymentToken.safeTransfer(msg.sender, refundAmount);
        
        emit RefundIssued(msg.sender, refundAmount);
    }
    
    /**
     * @dev Withdraw unsold IDO tokens (only owner)
     */
    function withdrawUnsoldTokens() external onlyOwner {
        require(block.timestamp > endTime, "IDO has not ended yet");
        
        uint256 unsoldTokens = idoToken.balanceOf(address(this)) - totalTokensSold;
        if (unsoldTokens > 0) {
            idoToken.safeTransfer(owner(), unsoldTokens);
        }
    }
    
    /**
     * @dev Withdraw collected payment tokens (only owner)
     */
    function withdrawCollectedPayments() external onlyOwner {
        require(block.timestamp > endTime, "IDO has not ended yet");
        require(totalPaymentCollected >= softCap, "Soft cap not reached");
        require(!globalRefundEnabled, "Cannot withdraw when global refund is enabled");
        
        uint256 balance = paymentToken.balanceOf(address(this));
        if (balance > 0) {
            paymentToken.safeTransfer(owner(), balance);
        }
    }
    
    /**
     * @dev Get IDO status information
     * @return _startTime Start time of the IDO
     * @return _endTime End time of the IDO
     * @return _softCap Soft cap in payment tokens
     * @return _hardCap Hard cap in payment tokens
     * @return _totalPaymentCollected Total payment tokens collected
     * @return _tokensClaimable Whether tokens are claimable
     * @return _refundsEnabled Whether refunds are enabled
     */
    function getIDOInfo() external view returns (
        uint256 _startTime,
        uint256 _endTime,
        uint256 _softCap,
        uint256 _hardCap,
        uint256 _totalPaymentCollected,
        bool _tokensClaimable,
        bool _refundsEnabled
    ) {
        return (
            startTime,
            endTime,
            softCap,
            hardCap,
            totalPaymentCollected,
            tokensClaimable,
            refundsEnabled || globalRefundEnabled
        );
    }
    
    /**
     * @dev Get user contribution information
     * @param user Address of the user
     * @return contribution User's contribution in payment tokens
     * @return tokenAmount Tokens amount to be received
     * @return claimed Whether user has claimed tokens
     * @return refunded Whether user has claimed a refund
     */
    function getUserInfo(address user) external view returns (
        uint256 contribution,
        uint256 tokenAmount,
        bool claimed,
        bool refunded
    ) {
        uint256 userContribution = contributions[user];
        uint256 userTokenAmount = (userContribution * (10**18)) / tokenPrice;
        
        return (
            userContribution,
            userTokenAmount,
            hasClaimed[user],
            hasRefunded[user]
        );
    }
}
