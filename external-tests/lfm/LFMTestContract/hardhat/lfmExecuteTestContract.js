const { ethers, upgrades } = require("hardhat");

/*
rum lfmDeployTestContract.js to deploy the contract and copy its address into CONTRACT_ADDR
npx hardhat compile
npx hardhat run --network privatenet hardhat/lfmExecuteTestContract.js
*/

async function main() {

  const CONTRACT_ADDR = "placeholder"; // TODO replace
  const CONTRACT_NAME = "LfmTestContract";

  const NATIVE_TX_GAS = 21000;
  const TEST_GAS_PRICE = BigInt(18000000000);

  const RECEIVER = "0x1637b6776c408929580ad68b68ef0c80c6398bab";

  _assert(CONTRACT_ADDR != "placeholder", "contract address not set");

  const [signer] = await ethers.getSigners();
  console.log("Deploying contracts with the account: ", signer.address);

  const contract = await ethers.getContractAt(CONTRACT_NAME, CONTRACT_ADDR);
  console.log("connected to contract at address: ", CONTRACT_ADDR);

  // A. pass funds to receiver directly (i.e. not via contract function)
  const prebalance = await _balance(CONTRACT_ADDR, "pre balance");

  const _value = ethers.parseUnits(String(1e8), 'wei'); // 0.1 gwei

  const tx3 = await signer.sendTransaction({ to: CONTRACT_ADDR, value: _value });
  await waitForReceipt(tx3, false);

  const postbalance = await _balance(CONTRACT_ADDR, "post balance");
  console.log("diff: ", postbalance - prebalance);
  _assert(postbalance - prebalance == _value);

  const tx4 = await contract.injectSomeCash({ value: _value });
  await waitForReceipt(tx4, false);

  const postinject = await _balance(CONTRACT_ADDR, "post inject");
  _assert(postinject - postbalance == _value);


  // B. invoke contract's naive transfer function - no gas limits
  const s_balance_pre = await _balance(signer, "signer pre");

  const tx6 = await contract.transferCoreTo(RECEIVER, _value);
  await waitForReceipt(tx6, false);

  const s_balance_post = await _balance(signer, "signer post");
  const gasCostForSigner = s_balance_pre - s_balance_post;
  _assert(gasCostForSigner > BigInt(NATIVE_TX_GAS) * TEST_GAS_PRICE); // i.e. contract function not mistaken to be native transfer

  console.log("signer gas cost: ", gasCostForSigner);

  const postnaive = await _balance(CONTRACT_ADDR, "post naive call");
  const _diff2 = postinject - postnaive;
  _assert(_diff2 == _value);


  // C. invoke contract's gas-limit function 
  const s_balance_pre2 = await _balance(signer, "signer pre");

  const tx7 = await contract.transferWithGasLimit(RECEIVER, _value);
  await waitForReceipt(tx7, false);

  const s_balance_post2 = await _balance(signer, "signer post");
  const gasCostForSigner2 = s_balance_pre2 - s_balance_post2;
  _assert(gasCostForSigner2 > BigInt(NATIVE_TX_GAS) * TEST_GAS_PRICE); // i.e. contract function not mistaken to be native transfer

  console.log("signer gas cost/2: ", gasCostForSigner2);
}


async function waitForReceipt(tx, print) {
  let receipt;
  do {
    receipt = await ethers.provider.getTransactionReceipt(tx.hash);
  } while (receipt == null);

  _assert(receipt.status == 1, "receipt indicate failure")

  if (print) {
    console.log("receipt:", receipt);
  } else {
    console.log("receipt received");
  }
}

function _assert(cond, err) {
  if (!cond) {
    throw new Error('assertion failed: ' + err);
  }
}

async function _balance(addr, msg) {
  const b = await ethers.provider.getBalance(addr);
  console.log(msg, b);
  return b;
}


main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });