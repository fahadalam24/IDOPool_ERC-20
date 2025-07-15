# Reset Hardhat blockchain and start over
Write-Host "Stopping any running Hardhat nodes..." -ForegroundColor Yellow
Stop-Process -Name "node" -ErrorAction SilentlyContinue

# Clean up cache
Write-Host "Cleaning Hardhat cache..." -ForegroundColor Yellow
if (Test-Path "./cache") {
    Remove-Item -Recurse -Force "./cache" -ErrorAction SilentlyContinue
}

# Clean artifacts
Write-Host "Cleaning artifacts..." -ForegroundColor Yellow
if (Test-Path "./artifacts") {
    Remove-Item -Recurse -Force "./artifacts" -ErrorAction SilentlyContinue
}

# Start fresh Hardhat node
Write-Host "Starting fresh Hardhat node..." -ForegroundColor Green
Start-Process powershell.exe -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot'; npx hardhat node"

# Wait for Hardhat node to start
Write-Host "Waiting for Hardhat node to start..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# Deploy contracts
Write-Host "Deploying contracts to fresh blockchain..." -ForegroundColor Green
npx hardhat run --network localhost scripts/deploy.js

Write-Host "Starting HTTP server for frontend..." -ForegroundColor Green
Start-Process powershell.exe -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot\frontend'; npx http-server -p 3000"

Write-Host "Setup complete! Access the frontend at http://localhost:3000" -ForegroundColor Cyan
Write-Host "IMPORTANT: In MetaMask, go to Settings > Advanced > Reset Account to clear the transaction history" -ForegroundColor Magenta
