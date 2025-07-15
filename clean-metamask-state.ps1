# Full MetaMask / Hardhat state reset script
Write-Host "=== COMPLETE METAMASK + HARDHAT STATE RESET ===" -ForegroundColor Green
Write-Host "Stopping any running Hardhat nodes..." -ForegroundColor Yellow
Get-Process -Name "node" -ErrorAction SilentlyContinue | Stop-Process -Force

# Clean up hardhat artifacts and cache
Write-Host "Cleaning Hardhat cache and artifacts..." -ForegroundColor Yellow
if (Test-Path "./cache") { Remove-Item -Recurse -Force "./cache" -ErrorAction SilentlyContinue }
if (Test-Path "./artifacts") { Remove-Item -Recurse -Force "./artifacts" -ErrorAction SilentlyContinue }

# Clean localStorage data that might be used by the frontend
Write-Host "Creating local storage clearing script..." -ForegroundColor Yellow
$clearLSHtml = @"
<!DOCTYPE html>
<html>
<head>
    <title>Clear Local Storage</title>
</head>
<body>
    <h1>Clearing MetaMask State...</h1>
    <script>
        // Clear all localStorage items
        localStorage.clear();
        console.log('LocalStorage cleared!');
        
        // Display success message
        document.body.innerHTML += '<p>Local storage has been cleared!</p>';
        document.body.innerHTML += '<p>You can close this window and return to the application.</p>';
    </script>
</body>
</html>
"@

# Write the HTML to a temporary file
$clearLSPath = "$PSScriptRoot\frontend\clear-storage.html"
Set-Content -Path $clearLSPath -Value $clearLSHtml

# Start server and open the clear-storage.html
Write-Host "Please open this URL in your browser to clear local storage: http://localhost:3001/clear-storage.html" -ForegroundColor Cyan
Start-Process powershell.exe -ArgumentList "-NoExit", "-Command", "cd '$PSScriptRoot\frontend'; npx http-server -p 3001"
Start-Process "http://localhost:3001/clear-storage.html"

Write-Host "INSTRUCTIONS:" -ForegroundColor Magenta
Write-Host "1. Visit the opened clear-storage page in your browser" -ForegroundColor White
Write-Host "2. Open MetaMask and go to Settings > Advanced > Reset Account" -ForegroundColor White
Write-Host "3. Wait 5 seconds, then press any key to continue..." -ForegroundColor White
Read-Host

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

Write-Host "===== RESET COMPLETE =====" -ForegroundColor Green
Write-Host "IMPORTANT: In MetaMask" -ForegroundColor Yellow
Write-Host "1. Disconnect and reconnect to the Localhost 8545 network" -ForegroundColor White
Write-Host "2. Verify you are on Chain ID 31337" -ForegroundColor White
Write-Host "3. You may need to reset your MetaMask account again (Settings > Advanced > Reset Account)" -ForegroundColor White
