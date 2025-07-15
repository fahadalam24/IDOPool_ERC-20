// Console test script for IDO contract interactions
async function main() {
  console.log("Testing IDO contract interactions");

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
  
  // Get IDO information
  const idoInfo = await idoPool.getIDOInfo();
  console.log("\nIDO Info:");
  console.log("- Start Time:", idoInfo[0] > 0 ? new Date(Number(idoInfo[0]) * 1000).toLocaleString() : "Not started");
  console.log("- End Time:", idoInfo[1] > 0 ? new Date(Number(idoInfo[1]) * 1000).toLocaleString() : "Not set");
  console.log("- Soft Cap:", ethers.formatEther(idoInfo[2]), "PAY tokens");
  console.log("- Hard Cap:", ethers.formatEther(idoInfo[3]), "PAY tokens");
  console.log("- Total Payment Collected:", ethers.formatEther(idoInfo[4]), "PAY tokens");
  console.log("- Tokens Claimable:", idoInfo[5]);
  console.log("- Refunds Enabled:", idoInfo[6]);

  // Check token balances
  console.log("\nToken Balances:");
  console.log("- IDO Pool contract IDO token balance:", ethers.formatEther(await idoToken.balanceOf(idoPoolAddress)));
  console.log("- IDO Pool contract payment token balance:", ethers.formatEther(await paymentToken.balanceOf(idoPoolAddress)));
  console.log("- User1 payment token balance:", ethers.formatEther(await paymentToken.balanceOf(user1.address)));
  console.log("- User2 payment token balance:", ethers.formatEther(await paymentToken.balanceOf(user2.address)));
}

main().catch(error => {
  console.error(error);
});
