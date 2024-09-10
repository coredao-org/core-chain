 // SPDX-License-Identifier: Apache2.0
pragma solidity ^0.8.19;

contract LfmTestContract {

    uint public val;

    constructor(address to) payable {
        if (msg.value > 0) {
            transferCoreTo(to, msg.value);
        }
    }

    fallback() external payable {} // allow receival of funds

    function injectSomeCash() external payable {}

    function simpleSet(uint256 newVal) external {
        val = newVal;
    }

    function transferCoreTo(address to, uint256 amount) external {
        require(address(this).balance >= amount, "not enough balance in contract");
        (bool ok, ) = payable(to).call{value: amount}("");
        require(ok, "transfer failed");
    }

    function transferWithGasLimit(address to, uint256 amount) external {
        (bool ok, ) = payable(to).call{value: amount, gas: 21000}("");
        require(ok, "transfer failed");
    }
}