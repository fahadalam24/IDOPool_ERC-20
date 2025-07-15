// Console-based testing script for IDO contract interactions
async function main() {
  console.log("========== IDO CONTRACT FUNCTIONALITY TEST ==========");

  // Get contract addresses
  const idoPoolAddress = '0x8A791620dd6260079BF849Dc5567aDC3F2FdC318';
  const idoTokenAddress = '0xa513E6E4b8f2a923D98304ec87F64353C4D5C853';
  const paymentTokenAddress = '0x2279B7A0a67DB372996a5FaB50D91eAA73d2eBe6';

  // Get contract instances
  const idoPool = await ethers.getContractAt("IDOPool", idoPoolAddress);
  const idoToken = await ethers.getContractAt("TestToken", idoTokenAddress);
  const paymentToken = await ethers.getContractAt("TestToken", paymentTokenAddress);

  // Get signers
  const [owner, user1, user2] = await ethers.getSigners();
  console.log("Owner address:", owner.address);
  console.log("User1 address:", user1.address);
  console.log("User2 address:", user2.address);

  // 1. Check initial balances
  console.log("\n1. CHECKING INITIAL BALANCES");
  console.log("- IDO Pool contract IDO token balance:", ethers.formatEther(await idoToken.balanceOf(idoPoolAddress)));
  console.log("- Owner payment token balance:", ethers.formatEther(await paymentToken.balanceOf(owner.address)));
  console.log("- User1 payment token balance:", ethers.formatEther(await paymentToken.balanceOf(user1.address)));
  
  // 2. Check initial IDO status
  const idoInfoInitial = await idoPool.getIDOInfo();
  console.log("\n2. CHECKING INITIAL IDO STATUS");
  console.log("- Start Time:", idoInfoInitial[0] > 0 ? new Date(Number(idoInfoInitial[0]) * 1000).toLocaleString() : "Not started");
  console.log("- End Time:", idoInfoInitial[1] > 0 ? new Date(Number(idoInfoInitial[1]) * 1000).toLocaleString() : "Not set");
  console.log("- Soft Cap:", ethers.formatEther(idoInfoInitial[2]), "PAY tokens");
  console.log("- Hard Cap:", ethers.formatEther(idoInfoInitial[3]), "PAY tokens");
  console.log("- Total Payment Collected:", ethers.formatEther(idoInfoInitial[4]), "PAY tokens");
  
  // 3. Start IDO
  console.log("\n3. STARTING IDO");
  if (idoInfoInitial[0] == 0) {
    const blockNum = await ethers.provider.getBlockNumber();
    const block = await ethers.provider.getBlock(blockNum);
    const currentTime = block.timestamp;
    
    const startTime = currentTime + 60; // Start in 1 minute
    const endTime = startTime + 3600; // End 1 hour after start
    
    console.log(`- Setting start time to: ${new Date(startTime * 1000).toLocaleString()}`);
    console.log(`- Setting end time to: ${new Date(endTime * 1000).toLocaleString()}`);
    
    const tx = await idoPool.startIDO(startTime, endTime);
    await tx.wait();
    console.log("- IDO Started successfully");
  } else {
    console.log("- IDO already started");
  }
  
  // 4. Approve and buy tokens as user1
  console.log("\n4. USER1 BUYING TOKENS");
  // First approve payment tokens
  const paymentAmount = ethers.parseEther("50000"); // 50,000 PAY tokens
  console.log(`- User1 approving ${ethers.formatEther(paymentAmount)} PAY tokens to be spent by IDO Pool`);
  const approveTx = await paymentToken.connect(user1).approve(idoPoolAddress, paymentAmount);
  await approveTx.wait();
  
  // Buy tokens
  console.log(`- User1 buying tokens for ${ethers.formatEther(paymentAmount)} PAY tokens`);
  try {
    const buyTx = await idoPool.connect(user1).buyTokens(paymentAmount);
    await buyTx.wait();
    console.log("- Tokens purchased successfully");
  } catch (error) {
    console.error("- Error buying tokens:", error.message);
  }
  
  // 5. Check user contribution
  console.log("\n5. CHECKING USER1 CONTRIBUTION");
  const user1Info = await idoPool.getUserInfo(user1.address);
  console.log("- Contribution amount:", ethers.formatEther(user1Info[0]), "PAY tokens");
  console.log("- Token amount to receive:", ethers.formatEther(user1Info[1]), "IDO tokens");
  console.log("- Has claimed tokens:", user1Info[2]);
  console.log("- Has claimed refund:", user1Info[3]);
  
  // 6. End the IDO
  console.log("\n6. ENDING IDO");
  try {
    const endTx = await idoPool.endIDO();
    await endTx.wait();
    console.log("- IDO ended successfully");
  } catch (error) {
    console.error("- Error ending IDO (might be already ended or not started yet):", error.message);
  }
  
  // 7. Enable token claims (if successful IDO)
  console.log("\n7. ENABLING TOKEN CLAIMS");
  try {
    const claimTx = await idoPool.enableTokenClaims();
    await claimTx.wait();
    console.log("- Token claims enabled successfully");
  } catch (error) {
    console.error("- Error enabling token claims (soft cap might not be reached):", error.message);
  }
  
  // 8. User claims tokens (if enabled)
  console.log("\n8. USER1 CLAIMING TOKENS");
  try {
    const claimTokensTx = await idoPool.connect(user1).claimTokens();
    await claimTokensTx.wait();
    console.log("- Tokens claimed successfully");
    console.log("- User1 IDO token balance:", ethers.formatEther(await idoToken.balanceOf(user1.address)));
  } catch (error) {
    console.error("- Error claiming tokens:", error.message);
  }
  
  // 9. Alternative path: enable refunds
  console.log("\n9. TESTING REFUND MECHANISM");
  try {
    if (user1Info[3] === false && user1Info[2] === false) { // If not refunded and not claimed
      console.log("- Enabling refunds (for demonstration)");
      await idoPool.setRefundsEnabled(true);
      console.log("- Setting refund window");
      const blockNum = await ethers.provider.getBlockNumber();
      const block = await ethers.provider.getBlock(blockNum);
      const currentTime = block.timestamp;
      await idoPool.setRefundWindow(currentTime + 3600); // 1 hour refund window
      
      console.log("- User2 attempting to claim refund");
      const user2ContributionBefore = await idoPool.getUserInfo(user2.address);
      if (user2ContributionBefore[0] > 0) {
        const refundTx = await idoPool.connect(user2).claimRefund();
        await refundTx.wait();
        console.log("- Refund claimed successfully");
      } else {
        console.log("- User2 has no contribution to refund");
      }
    } else {
      console.log("- User1 already claimed tokens or refund, skipping refund test");
    }
  } catch (error) {
    console.error("- Error in refund process:", error.message);
  }
  
  // 10. Admin withdraws funds (if applicable)
  console.log("\n10. ADMIN WITHDRAWING FUNDS");
  try {
    const withdrawTx = await idoPool.withdrawCollectedPayments();
    await withdrawTx.wait();
    console.log("- Funds withdrawn successfully");
    console.log("- Admin payment token balance after withdrawal:", ethers.formatEther(await paymentToken.balanceOf(owner.address)));
  } catch (error) {
    console.error("- Error withdrawing funds (soft cap might not be reached):", error.message);
  }
  
  // 11. Final IDO status
  const idoInfoFinal = await idoPool.getIDOInfo();
  console.log("\n11. FINAL IDO STATUS");
  console.log("- Total Payment Collected:", ethers.formatEther(idoInfoFinal[4]), "PAY tokens");
  console.log("- Tokens Claimable:", idoInfoFinal[5]);
  console.log("- Refunds Enabled:", idoInfoFinal[6]);

  console.log("\n========== TEST COMPLETED ==========");
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });
