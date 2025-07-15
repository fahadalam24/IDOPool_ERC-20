// Script for deploying the IDOPool contract
const { ethers } = require("hardhat");
const fs = require('fs');
const path = require('path');

async function main() {
  console.log("Deploying contracts...");
  // Deploy IDO token
  const TestToken = await ethers.getContractFactory("TestToken");
  const idoToken = await TestToken.deploy(
    "IDO Token", 
    "IDO", 
    18, 
    15000000 // 15 million tokens (increased to match test setup)
  );
  await idoToken.deploymentTransaction().wait();
  console.log(`IDO Token deployed to: ${idoToken.target}`);

  // Deploy payment token
  const paymentToken = await TestToken.deploy(
    "Payment Token", 
    "PAY", 
    18, 
    10000000 // 10 million tokens
  );
  await paymentToken.deploymentTransaction().wait();
  console.log(`Payment Token deployed to: ${paymentToken.target}`);

  // Deploy IDO Pool
  const tokenPrice = ethers.parseEther("0.01"); // 1 IDO token = 0.01 PAY tokens
  const softCap = ethers.parseEther("100000"); // 100,000 PAY tokens
  const hardCap = ethers.parseEther("500000"); // 500,000 PAY tokens

  const IDOPool = await ethers.getContractFactory("IDOPool");
  const idoPool = await IDOPool.deploy(
    idoToken.target,
    paymentToken.target,
    tokenPrice,
    softCap,
    hardCap
  );
  await idoPool.deploymentTransaction().wait();
  console.log(`IDO Pool deployed to: ${idoPool.target}`);

  // Verify IDO Pool initialization
  const deployedIdoToken = await idoPool.idoToken();
  const deployedOwner = await idoPool.owner();
  console.log(`IDO Pool initialized with IDO Token: ${deployedIdoToken}, Owner: ${deployedOwner}`);

  // Transfer IDO tokens to the IDO Pool
  const idoTokensForPool = ethers.parseEther("12000000"); // 12,000,000 IDO tokens for the pool
  await idoToken.transfer(idoPool.target, idoTokensForPool);
  console.log(`Transferred ${ethers.formatEther(idoTokensForPool)} IDO tokens to the IDO Pool`);
  
  // Distribute payment tokens to test accounts
  const accounts = await ethers.getSigners();
  const userPaymentAmount = ethers.parseEther("150000"); // 150,000 payment tokens per user
  
  for (let i = 1; i < 4; i++) {
    await paymentToken.transfer(accounts[i].address, userPaymentAmount);
    console.log(`Transferred ${ethers.formatEther(userPaymentAmount)} PAY tokens to ${accounts[i].address}`);
  }

  console.log("Deployment complete!");

  // Update frontend/index.html
  const frontendPath = path.join(__dirname, '../frontend/index.html');
  let frontendContent = fs.readFileSync(frontendPath, 'utf8');

  frontendContent = frontendContent.replace(
    /const idoPoolAddress = '.*?';/,
    `const idoPoolAddress = '${idoPool.target}';`
  );
  frontendContent = frontendContent.replace(
    /const idoTokenAddress = '.*?';/,
    `const idoTokenAddress = '${idoToken.target}';`
  );
  frontendContent = frontendContent.replace(
    /const paymentTokenAddress = '.*?';/,
    `const paymentTokenAddress = '${paymentToken.target}';`
  );
  fs.writeFileSync(frontendPath, frontendContent, 'utf8');

  // Update scripts/console-test.js
  const consoleTestPath = path.join(__dirname, 'console-test.js');
  let consoleTestContent = fs.readFileSync(consoleTestPath, 'utf8');
  consoleTestContent = consoleTestContent.replace(
    /const idoPoolAddress = '.*?';/,
    `const idoPoolAddress = '${idoPool.target}';`
  );
  consoleTestContent = consoleTestContent.replace(
    /const idoTokenAddress = '.*?';/,
    `const idoTokenAddress = '${idoToken.target}';`
  );
  consoleTestContent = consoleTestContent.replace(
    /const paymentTokenAddress = '.*?';/,
    `const paymentTokenAddress = '${paymentToken.target}';`
  );
  fs.writeFileSync(consoleTestPath, consoleTestContent, 'utf8');

  // Update scripts/ido-test.js
  const idoTestPath = path.join(__dirname, 'ido-test.js');
  let idoTestContent = fs.readFileSync(idoTestPath, 'utf8');
  idoTestContent = idoTestContent.replace(
    /const idoPoolAddress = '.*?';/,
    `const idoPoolAddress = '${idoPool.target}';`
  );
  idoTestContent = idoTestContent.replace(
    /const idoTokenAddress = '.*?';/,
    `const idoTokenAddress = '${idoToken.target}';`
  );
  idoTestContent = idoTestContent.replace(
    /const paymentTokenAddress = '.*?';/,
    `const paymentTokenAddress = '${paymentToken.target}';`
  );
  fs.writeFileSync(idoTestPath, idoTestContent, 'utf8');

  console.log("All relevant files updated with new contract addresses.");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
