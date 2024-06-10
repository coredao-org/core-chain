const { ethers, upgrades } = require("hardhat");

/*
npx hardhat compile
npx hardhat run --network privatenet hardhat/lfmDeployTestContract.js
*/

async function main() {
  const CONTRACT_NAME = "LfmTestContract";
  const LfmTestContract = await ethers.getContractFactory(CONTRACT_NAME);
  contract = await LfmTestContract.deploy();
  const contractAddr = await contract.getAddress();
  console.log("\ncontract address (to be copied into lfmExecuteTestContract.js): ", contractAddr);
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });