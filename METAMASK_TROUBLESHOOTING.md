# MetaMask + Hardhat Troubleshooting Guide

This guide provides solutions for common issues when working with MetaMask and local Hardhat blockchains.

## "Invalid Block Tag" Error

### PROBLEM
When connecting to contracts, you see errors like:
```
Received invalid block tag 28. Latest block number is 7
```

### ROOT CAUSE
- MetaMask caches blockchain state, including block numbers
- When you restart Hardhat, it creates a new blockchain starting from block 0
- MetaMask still tries to access old blocks that don't exist anymore

### SOLUTION

#### Method 1: Reset Account in MetaMask
1. Open MetaMask
2. Go to Settings > Advanced
3. Click "Reset Account"
4. Refresh your application page

#### Method 2: Run the Clean State Script
```powershell
cd "d:\Blockchain Assignment SmartContract"
.\clean-metamask-state.ps1
```

#### Method 3: Complete Manual Reset
1. Close all browser windows
2. Stop all Hardhat nodes (`Get-Process -Name "node" | Stop-Process -Force`)
3. Delete Hardhat cache and artifacts folders
4. Restart Hardhat node (`npx hardhat node`)
5. Redeploy contracts (`npx hardhat run --network localhost scripts/deploy.js`)
6. Open the clear-storage.html page in your browser 
7. Open MetaMask and go to Settings > Advanced > Reset Account
8. Restart your browser
9. Reconnect to the local network in MetaMask

## Connection Issues

### PROBLEM
MetaMask shows connected, but contract calls fail

### SOLUTIONS

1. **Verify Chain ID**:
   - Ensure you're connected to Hardhat (Chain ID 31337)
   - RPC URL should be http://127.0.0.1:8545 or http://localhost:8545

2. **Check Contract Addresses**:
   - Verify that contract addresses in frontend match what was deployed
   - Current addresses should be:
     - IDO Pool: 0x9fE46736679d2D9a65F0992F2272dE9f3c7fa6e0
     - IDO Token: 0x5FbDB2315678afecb367f032d93F642f64180aa3
     - Payment Token: 0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512

3. **Test Account Setup**:
   - Import an account using the private key from Hardhat output:
   - Account #0: 0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266
   - Private Key: 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80

## Additional Tips

1. **Browser Cache**:
   - Try using incognito/private mode
   - Clear your browser cache and cookies

2. **Hardhat Node**:
   - Use `npx hardhat node --hostname 0.0.0.0` to allow connections from other devices
   - Check for error messages in the Hardhat console

3. **Frontend Reset**:
   - Use the clear-storage.html tool to reset browser storage
   - Restart the local HTTP server

4. **MetaMask Network Settings**:
   - Delete and re-add the Hardhat network in MetaMask
   - Network Name: Hardhat Network
   - RPC URL: http://127.0.0.1:8545/
   - Chain ID: 31337
   - Currency Symbol: ETH

If all else fails, completely uninstall and reinstall MetaMask, then re-import your accounts.
