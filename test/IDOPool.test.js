const { expect } = require("chai");
const hre = require("hardhat");
const { ethers } = hre;

describe("IDO Pool Contract", function () {
  // Variables used across tests
  let IDOPool;
  let idoPool;
  let TestToken;
  let idoToken;
  let paymentToken;
  let owner;
  let user1;
  let user2;
  let user3;
  let tokenPrice;
  let softCap;
  let hardCap;
  
  // Time manipulation helper
  async function increaseTime(seconds) {
    await ethers.provider.send("evm_increaseTime", [seconds]);
    await ethers.provider.send("evm_mine");
  }
  
  async function getCurrentBlockTimestamp() {
    const blockNum = await ethers.provider.getBlockNumber();
    const block = await ethers.provider.getBlock(blockNum);
    return block.timestamp;
  }

  beforeEach(async function () {
    // Get signers
    [owner, user1, user2, user3] = await ethers.getSigners();
      // Deploy test tokens
    TestToken = await ethers.getContractFactory("TestToken");
    idoToken = await TestToken.deploy("IDO Token", "IDO", 18, 15000000); // Increased from 1,000,000 to 15,000,000
    paymentToken = await TestToken.deploy("Payment Token", "PAY", 18, 10000000);
    
    // Set price and caps
    tokenPrice = ethers.parseEther("0.01"); // 1 IDO token = 0.01 PAY tokens
    softCap = ethers.parseEther("100000"); // 100,000 PAY tokens
    hardCap = ethers.parseEther("500000"); // 500,000 PAY tokens
    
    // Deploy IDO pool contract
    IDOPool = await ethers.getContractFactory("IDOPool");
    idoPool = await IDOPool.deploy(
      await idoToken.getAddress(),
      await paymentToken.getAddress(),
      tokenPrice,
      softCap,
      hardCap
    );
      // Transfer IDO tokens to the pool
    const idoTokensForPool = ethers.parseEther("12000000"); // 12,000,000 IDO tokens (increased from 500,000)
    await idoToken.transfer(await idoPool.getAddress(), idoTokensForPool);
      // Distribute payment tokens to users for testing
    const userPaymentAmount = ethers.parseEther("150000"); // Increased from 50000 to 150000
    await paymentToken.transfer(user1.address, userPaymentAmount);
    await paymentToken.transfer(user2.address, userPaymentAmount);
    await paymentToken.transfer(user3.address, userPaymentAmount);
    
    // Approve IDO pool to spend payment tokens
    await paymentToken.connect(user1).approve(await idoPool.getAddress(), ethers.MaxUint256);
    await paymentToken.connect(user2).approve(await idoPool.getAddress(), ethers.MaxUint256);
    await paymentToken.connect(user3).approve(await idoPool.getAddress(), ethers.MaxUint256);
  });

  describe("Contract Initialization", function () {
    it("Should correctly initialize the IDO pool", async function () {
      expect(await idoPool.idoToken()).to.equal(await idoToken.getAddress());
      expect(await idoPool.paymentToken()).to.equal(await paymentToken.getAddress());
      expect(await idoPool.tokenPrice()).to.equal(tokenPrice);
      expect(await idoPool.softCap()).to.equal(softCap);
      expect(await idoPool.hardCap()).to.equal(hardCap);
      expect(await idoPool.owner()).to.equal(owner.address);
      expect(await idoToken.balanceOf(await idoPool.getAddress())).to.equal(ethers.parseEther("12000000"));
    });
  });

  describe("Starting and Ending IDO", function () {
    it("Should start the IDO correctly", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      
      const idoInfo = await idoPool.getIDOInfo();
      expect(idoInfo[0]).to.equal(startTime);
      expect(idoInfo[1]).to.equal(endTime);
    });
    
    it("Should end the IDO early", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      await idoPool.endIDO();
      const idoInfo = await idoPool.getIDOInfo();
      expect(idoInfo[1]).to.be.lessThan(endTime);
    });
  });

  describe("Token Purchase", function () {
    it("Should allow users to buy tokens", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      const paymentAmount = ethers.parseEther("1000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // Expected token amount: (1000 * 10^18) / (0.01 * 10^18) = 100,000 tokens
      const expectedTokens = paymentAmount / tokenPrice * ethers.parseEther("1");
      
      const userInfo = await idoPool.getUserInfo(user1.address);
      expect(userInfo[0]).to.equal(paymentAmount); // contribution
      expect(userInfo[1].toString()).to.equal(expectedTokens.toString()); // token amount
    });
    
    it("Should reject purchases after IDO ends", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(4000); // Move past end time
      
      const paymentAmount = ethers.parseEther("1000");
      await expect(idoPool.connect(user1).buyTokens(paymentAmount)).to.be.revertedWith("IDO has ended");
    });
  });

  describe("Token Claiming", function () {
    it("Should allow users to claim tokens after enabled by admin", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // User1 buys tokens
      const paymentAmount = ethers.parseEther("110000"); // Above soft cap
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // End IDO
      await increaseTime(4000);
      
      // Admin enables token claims
      await idoPool.enableTokenClaims();
      
      // User claims tokens
      await idoPool.connect(user1).claimTokens();
        // Expected token amount: (110000 * 10^18) / (0.01 * 10^18) = 11,000,000 tokens
      const expectedTokens = (paymentAmount * BigInt(10**18)) / tokenPrice;
      
      // Check user now has tokens
      expect(await idoToken.balanceOf(user1.address)).to.equal(expectedTokens);
      
      // Check user has claimed flag is set
      const userInfo = await idoPool.getUserInfo(user1.address);
      expect(userInfo[2]).to.be.true; // claimed flag
    });
  });

  describe("Refund Mechanisms", function () {
    it("Should allow users to get refunds when soft cap not met", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      const refundEndTime = endTime + 3600; // 1 hour after IDO end
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // User1 buys tokens (below soft cap)
      const paymentAmount = ethers.parseEther("50000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // End IDO
      await increaseTime(4000);
      
      // Admin enables refunds and sets refund window
      await idoPool.setRefundWindow(refundEndTime);
      await idoPool.setRefundsEnabled(true);
      
      // User claims refund
      const balanceBefore = await paymentToken.balanceOf(user1.address);
      await idoPool.connect(user1).claimRefund();
      const balanceAfter = await paymentToken.balanceOf(user1.address);
        // Verify user got refund
      expect(balanceAfter - balanceBefore).to.equal(paymentAmount);
      
      // Check user has refunded flag set
      const userInfo = await idoPool.getUserInfo(user1.address);
      expect(userInfo[3]).to.be.true; // refunded flag
    });
    
    it("Should allow admin to trigger global refund", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // Multiple users buy tokens
      const paymentAmount = ethers.parseEther("50000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      await idoPool.connect(user2).buyTokens(paymentAmount);
      
      // Admin enables global refund
      await idoPool.enableGlobalRefund();
      expect(await idoPool.globalRefundEnabled()).to.be.true;
      expect(await idoPool.refundsEnabled()).to.be.true;
      
      // User1 claims refund
      const balanceBefore = await paymentToken.balanceOf(user1.address);
      await idoPool.connect(user1).claimRefund();
      const balanceAfter = await paymentToken.balanceOf(user1.address);
        // Verify user got refund
      expect(balanceAfter - balanceBefore).to.equal(paymentAmount);
    });
    
    it("Should prevent refunds after claiming tokens", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // User1 buys tokens (above soft cap to enable token claims)
      const paymentAmount = ethers.parseEther("110000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // End IDO
      await increaseTime(4000);
      
      // Admin enables token claims
      await idoPool.enableTokenClaims();
      
      // User claims tokens
      await idoPool.connect(user1).claimTokens();
      
      // Admin tries to enable refunds (which should work but not affect claimed tokens)
      await idoPool.setRefundsEnabled(true);
      
      // User attempts to claim refund
      await expect(idoPool.connect(user1).claimRefund()).to.be.revertedWith("Cannot refund after claiming tokens");
    });
  });

  describe("Admin Functions", function () {
    it("Should allow admin to withdraw collected payments after soft cap met", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // User1 buys tokens (above soft cap)
      const paymentAmount = ethers.parseEther("110000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // End IDO
      await increaseTime(4000);
      
      // Admin withdraws collected payments
      const balanceBefore = await paymentToken.balanceOf(owner.address);
      await idoPool.withdrawCollectedPayments();
      const balanceAfter = await paymentToken.balanceOf(owner.address);
        // Verify admin got payments
      expect(balanceAfter - balanceBefore).to.equal(paymentAmount);
    });
    
    it("Should prevent admin from withdrawing if soft cap not met", async function () {
      const currentTime = await getCurrentBlockTimestamp();
      const startTime = currentTime + 100;
      const endTime = startTime + 3600; // 1 hour after start
      
      await idoPool.startIDO(startTime, endTime);
      await increaseTime(150); // Move past start time
      
      // User1 buys tokens (below soft cap)
      const paymentAmount = ethers.parseEther("50000");
      await idoPool.connect(user1).buyTokens(paymentAmount);
      
      // End IDO
      await increaseTime(4000);
      
      // Admin attempts to withdraw collected payments
      await expect(idoPool.withdrawCollectedPayments()).to.be.revertedWith("Soft cap not reached");
    });
  });
});
