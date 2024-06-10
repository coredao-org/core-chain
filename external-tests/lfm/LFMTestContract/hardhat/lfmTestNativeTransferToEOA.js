const { ethers } = require("hardhat")

/*
npx hardhat compile
npx hardhat run --network privatenet hardhat/lfmTestNativeTransferToEOA.js
*/

async function main() {

  const RECEIVER = "0x1637b6776c408929580ad68b68ef0c80c6398bab";
  const PASSED_ETH = BigInt(60000000);
  const NATIVE_TX_GAS = 21000;
  const TEST_GAS_PRICE = BigInt(18000000000);

  const [signer] = await ethers.getSigners();
  console.log('===> signer address (should be 0x0df0E17c921731419b78A35Bf7bA62DcC4B8024B) actual : ', signer.address);

  const _value = ethers.parseUnits(String(PASSED_ETH), 'wei'); 
  console.log('===> _value: ', _value);

  const receiver_pre = await ethers.provider.getBalance(RECEIVER);
  const sender_pre = await ethers.provider.getBalance(signer.address);

  // pass eth to an EOA receiver
  const tx = await signer.sendTransaction({
    to: RECEIVER,
    value: _value
  });
  await _waitForReceipt(tx.hash);

  const receiver_post = await ethers.provider.getBalance(RECEIVER);
  const sender_post = await ethers.provider.getBalance(signer.address);

  console.log("Transaction hash:", tx.hash);

  const spentGas = BigInt(sender_pre - sender_post);
  console.log("Sender pre:", sender_pre, " post:", sender_post, " diff: ", spentGas);

  const _received = receiver_post - receiver_pre;
  console.log("Receiver pre:", receiver_pre, " post:", receiver_post, " diff: ", _received);

  // verify eth received by EOA
  _assert(PASSED_ETH == _received, "transfer error")

  // verify discounted gas fees to sender
  _assert(spentGas - BigInt(NATIVE_TX_GAS) * TEST_GAS_PRICE - PASSED_ETH == 0, "gas reduction error")

}

async function _waitForReceipt(txhash) {
  let receipt;
  do {
    receipt = await ethers.provider.getTransactionReceipt(txhash);
  } while (receipt == null);

  _assert(receipt.status == 1, "receipt: failed status"); // verify success
}


function _assert(cond, err) {
  if (!cond) {
    throw new Error('assertion failed with error: ', err);
  }
}

main()
  .then(() => process.exit(0))
  .catch(error => {
    console.error(error);
    process.exit(1);
  });
