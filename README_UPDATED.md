# IDO Pool with ERC-20 Payment Integration

This project implements a smart contract for an Initial DEX Offering (IDO) pool that accepts payments in ERC-20 tokens and includes robust refund mechanisms.

## Features

- Accepts payments in user-defined ERC-20 tokens (not native ETH)
- Allows users to buy IDO tokens using the accepted payment token
- Maintains records of individual contributions
- Includes refund mechanisms for users and a global refund option for admin
- Admin can configure parameters like start time, end time, and refund window
- Secure against reentrancy attacks and other vulnerabilities
- Uses OpenZeppelin libraries for security best practices

## Implementation Details

### Contracts

1. `IDOPool.sol` - Main contract for the IDO pool functionality
   - Accepts ERC20 tokens as payment
   - Implements refund mechanisms
   - Handles token distribution
   - Includes admin controls for IDO management
   
2. `TestToken.sol` - ERC-20 token contract for testing purposes
   - Simple ERC20 implementation for both IDO token and payment token

### Security Measures

- Implemented reentrancy protection using OpenZeppelin's ReentrancyGuard
- Used SafeERC20 library to prevent common ERC20 token issues
- Input validation on all parameters
- Proper access control using Ownable pattern
- Validation of ERC20 token interfaces
- State machine pattern to manage IDO lifecycle

## Setup and Installation

### Prerequisites

- Node.js (v14+ recommended)
- npm or yarn package manager

### Installation

Clone the repository and install the dependencies:

```bash
git clone <repository-url>
cd <project-folder>
npm install
```

### Compile Contracts

Compile the Solidity smart contracts:

```bash
npx hardhat compile
```

### Running Tests

Run the test suite to verify the contract functionality:

```bash
npx hardhat test
```

### Deployment

Deploy the contracts to a local network (for development):

```bash
npx hardhat node
npx hardhat run scripts/deploy.js --network localhost
```

For deployment to a testnet or mainnet, update the `hardhat.config.js` with appropriate network settings and private keys.

## Usage Guide

### Creating a New IDO Pool

1. Deploy your IDO token (or use an existing ERC20 token)
2. Deploy a payment token (or use an existing ERC20 token like USDC/USDT)
3. Deploy the IDO Pool with the following parameters:
   - IDO token address
   - Payment token address
   - Token price (in payment token units)
   - Soft cap (minimum fundraising goal)
   - Hard cap (maximum fundraising limit)
4. Transfer sufficient IDO tokens to the IDO Pool contract

### IDO Lifecycle

#### As an Admin/Contract Owner

1. **Start the IDO**: Call `startIDO(startTime, endTime)` with appropriate timestamps.
2. **Configure refund window**: Call `setRefundWindow(refundEndTime)` to set the deadline for claiming refunds.
3. **End IDO early (if needed)**: Call `endIDO()` to end the IDO before the scheduled end time.
4. **After IDO ends**:
   - If soft cap is reached: Call `enableTokenClaims()` to allow users to claim tokens.
   - If soft cap is not reached: Call `setRefundsEnabled(true)` to allow users to claim refunds.
5. **Emergency scenario**: Call `enableGlobalRefund()` to allow all participants to withdraw funds.
6. **Withdraw funds**: After a successful IDO, call `withdrawCollectedPayments()` to withdraw payment tokens.
7. **Withdraw unsold tokens**: Call `withdrawUnsoldTokens()` to recover any unsold IDO tokens.

#### As a User/Investor

1. **Buy tokens**: During the active IDO period:
   - Approve the IDO Pool contract to spend your payment tokens
   - Call `buyTokens(amount)` with the amount of payment tokens to spend
2. **After IDO ends**:
   - If IDO succeeds: Call `claimTokens()` to receive your purchased IDO tokens.
   - If IDO fails: Call `claimRefund()` to get your payment tokens back when refunds are enabled.

## Contract Information

### IDOPool Contract

**State Variables**:
- `idoToken`: The token being sold in the IDO
- `paymentToken`: The token accepted for payment
- `startTime`: Start time of the IDO
- `endTime`: End time of the IDO
- `softCap`: Minimum amount needed for the IDO to be considered successful
- `hardCap`: Maximum amount that can be collected
- `tokenPrice`: Price of 1 IDO token in payment token units
- `refundEndTime`: End time for the refund period
- `refundsEnabled`: Flag to enable/disable refunds
- `globalRefundEnabled`: Flag to enable global refund
- `tokensClaimable`: Flag to indicate if tokens can be claimed

**Key Functions**:
- `startIDO(startTime, endTime)`: Start the IDO
- `buyTokens(amount)`: Buy IDO tokens with payment tokens
- `claimTokens()`: Claim purchased IDO tokens
- `claimRefund()`: Claim refund for contributed payment tokens
- `getIDOInfo()`: Get IDO status information
- `getUserInfo(address)`: Get user contribution information

## License

This project is licensed under the MIT License
