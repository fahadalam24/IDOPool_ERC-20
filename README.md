# IDO Pool Smart Contract

This is a Solidity-based smart contract implementation of an Initial DEX Offering (IDO) pool that accepts payments in ERC-20 tokens and provides refund mechanisms.

## Features

- **ERC-20 Payment Support**: Accepts payments in the form of ERC-20 tokens instead of native ETH
- **Token Purchase**: Users can buy IDO tokens using specified payment tokens
- **Refund Mechanisms**: 
  - Automatic refunds if soft cap is not reached
  - Global admin refund option for emergency situations
- **Admin Controls**:
  - Start/end IDO
  - Enable token claims
  - Withdraw funds after successful IDO
  - Withdraw unsold tokens
- **Security Features**:
  - Reentrancy protection
  - Ownership controls
  - ERC20 safety mechanisms

## Smart Contract Architecture

### IDOPool.sol

The main contract that manages the IDO process with the following key components:

- Token configuration (IDO token and payment token)
- IDO parameters (soft cap, hard cap, token price, start/end times)
- User contribution tracking
- Token claiming functionality
- Refund mechanisms
- Admin controls

### TestToken.sol

A simple ERC-20 token implementation used for testing the IDO and payment functionality.

## Test Suite

The test suite covers all critical functionality:

- Contract initialization
- Starting and ending IDO
- Token purchase
- Token claiming
- Refund mechanisms (soft cap not met, global refund)
- Admin functions

## Getting Started

### Prerequisites

- Node.js v14+ and npm
- MetaMask or another web3 wallet

### Installation

1. Clone the repository
```bash
git clone <repository-url>
cd Blockchain-Assignment-SmartContract
```

2. Install dependencies
```bash
npm install
```

3. Run tests
```bash
npx hardhat test
```

### Deployment

1. Start a local Hardhat node
```bash
npx hardhat node
```

2. Deploy the contracts
```bash
npx hardhat run scripts/deploy.js --network localhost
```

3. Start the frontend
```bash
node server.js
```

4. Access the frontend at http://localhost:3001

**Note**: PowerShell users should refer to the `POWERSHELL_COMMANDS.md` file for PowerShell-compatible commands, as PowerShell doesn't support the `&&` operator for command chaining.

## Frontend Interface

The frontend provides a user-friendly interface for interacting with the IDO pool:

### User Actions
- Connect wallet
- View IDO information
- Buy tokens
- Claim purchased tokens
- Claim refunds (when applicable)

### Admin Actions
- Manage IDO lifecycle (start/end)
- Enable token claims
- Enable refunds
- Withdraw collected payments
- Withdraw unsold tokens

## Workflow

1. Admin deploys the contracts and transfers IDO tokens to the pool
2. Admin starts the IDO with specified start/end times
3. Users buy tokens during the IDO period
4. After IDO ends:
   - If soft cap is met: Admin enables token claims, users claim tokens
   - If soft cap is not met: Admin enables refunds, users claim refunds

## Security Considerations

- **Reentrancy Protection**: All external calls are protected against reentrancy attacks
- **Input Validation**: All function inputs are validated
- **SafeERC20**: Uses OpenZeppelin's SafeERC20 for safe token transfers
- **Access Control**: Admin functions are protected with Ownable
- **Token Verification**: Validates token interfaces during contract initialization

## License

This project is licensed under the MIT License - see the LICENSE file for details.