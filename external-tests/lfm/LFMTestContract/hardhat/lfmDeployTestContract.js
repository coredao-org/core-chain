const { ethers, upgrades } = require("hardhat");

/*
npx hardhat compile
npx hardhat run --network privatenet hardhat/lfmDeployTestContract.js
*/

async function main() {
  const RECEIVER = "0x1637b6776c408929580ad68b68ef0c80c6398bab";
  const CONTRACT_NAME = "LfmTestContract";

  const [signer] = await ethers.getSigners();
  console.log("Deploying contract with the account: ", signer.address);
  const _value = ethers.parseUnits(String(1e8), 'wei'); // 0.1 gwei

  const LfmTestContract = await ethers.getContractFactory(CONTRACT_NAME);
  // pass eth to CTOR so to invoke transfer before contract is deployed resulting in code-size==0
  const contract = await LfmTestContract.deploy(RECEIVER, { from: signer.address, value: _value });
  const contractAddr = await contract.getAddress();
  console.log("\ncontract address (to be copied into lfmExecuteTestContract.js): ", contractAddr);
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });