package lfm

import (
	"bytes"
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
)


const (
	historicalMeanGasPrice = 35783571428 //wei
	historicalSDGasPrice = 849870638 //wei
	maxDiscountedGasPrice  = historicalMeanGasPrice + historicalSDGasPrice/2 // =36208506747wei

	coreTransferGasAmount = 21000 // hard-coded gas amount

	functionSelectorSize = 4
	transferDataLen = 4+32+32  // transfer(address,uint256): 4 bytes for func ID, 32 bytes for address (20 bytes padded to 32), 32 bytes for amount
	transferFromDataLen = 4+32+32+32  // transferFrom(address,address,uint256): 4 bytes for func ID, 32 bytes for from address, 32 bytes for to address, 32 bytes for amount
	approveDataLen = 4+32+32// approve(address,uint256): 4 bytes for func ID, 32 bytes for the spender address (padded), 32 bytes for the amount

	supportWhitelistedErc20s = true
	useDebugDiscountedGasPrice = false
)

// erc20 function types eligible for discount
type funcType uint

const (
	ftype_none funcType = iota
	ftype_transfer
	ftype_transferFrom
	ftype_approve
)

var (
	// erc20's transfer: the first 4 bytes of the keccak256 hash of "transfer(address,uint256)"
	transferFunction = []byte{0xa9, 0x05, 0x9c, 0xbb}

	// erc20's transferFrom: the first 4 bytes of the keccak256 hash of "transferFrom(address,address,uint256)"
	transferFromFunction = []byte{0x23, 0xb8, 0x72, 0xdd}

	// erc20's approve: the first 4 bytes of the keccak256 hash of "approve(address,uint256)"
	approveFunction = []byte{0x09, 0x5e, 0xa7, 0xb3}

	debugDiscountedGasPrice = new(big.Int).SetUint64(18_000_000_000)

	errNotFunctionInvocation = errors.New("tx is not a function invocation")
)

// white-listed erc20 map
var (
	btcLstContract = common.HexToAddress("0xTBD")

	internalErc20Whitelist = map[common.Address]bool{
		btcLstContract: true,
	}
)


func AdjustGasPriceForEstimation(_networkGasPrice *hexutil.Big, _gasAmount *hexutil.Uint64, _value *hexutil.Big, to *common.Address, data []byte) *hexutil.Big {
	if _networkGasPrice == nil || _gasAmount == nil || _value == nil {
		return _networkGasPrice
	}
	networkGasPrice := (*big.Int)(_networkGasPrice)
	gasAmount := uint64(*_gasAmount)
	value := (*big.Int)(_value)
	adjusted := AdjustGasPrice(gasAmount, value, to, data, networkGasPrice)
	return (*hexutil.Big)(adjusted)
}

func AdjustGasPrice(gasAmount uint64, value *big.Int, to *common.Address, data []byte, networkGasPrice *big.Int) *big.Int {
	// apply gas-price discount if tx is a 'native' transfer of either Core or whitelisted erc20
	if isCoreTransferTx(gasAmount, value, len(data)) {
		return discountCoreTransferTx(networkGasPrice)
	}

	if ftype := isErc20TransferTx(to, data); ftype != ftype_none {
		return discountErc20TransferTx(networkGasPrice, ftype, gasAmount)
	}

	return networkGasPrice
}


func isCoreTransferTx(gasAmount uint64, value *big.Int, dataLen int) bool {
	// requires (a) EOA destination (b) value > 0 (c) empty data indicating not a smart-contract
	// invocation and (d) total gasAmount == 21k so to rule out transfer into a smart contract (and not EOA)
	hasValue := value != nil && value.Sign() > 0
	eoaToAddress := gasAmount == coreTransferGasAmount
	isCoreTransfer := hasValue && eoaToAddress && dataLen == 0
	return isCoreTransfer
}

func discountCoreTransferTx(networkGasPrice *big.Int) *big.Int {
	// apply gas-price discount for Core transfer tx
	if useDebugDiscountedGasPrice {
		log.Debug("@lfm: debug-mode core transfer discount", "networkGasPrice", networkGasPrice, "debugGasPrice", debugDiscountedGasPrice)
		return debugDiscountedGasPrice
	}
	discountedGasPrice := calcCoreTransferDiscountedGasPrice(networkGasPrice)
	log.Debug("@lfm: core transfer discount", "networkGasPrice", networkGasPrice, "discounted-gas-price", discountedGasPrice)
	return discountedGasPrice
}

func calcCoreTransferDiscountedGasPrice(networkGasPrice *big.Int)  *big.Int {
	// calc discounted gas price for native Core transfer tx
	return internalCalcDiscountedGasPrice(networkGasPrice)
}

/**
  discrete steps for the sigmoid function sig(x) = 1 / (1 + e^(-3 * (x - 0.8)))
  where x denotes the historical mean, slightly modified for simplicity
  using integer steps so to avoid potential floating point inconsistencies between nodes
 */
func internalCalcDiscountedGasPrice(networkGasPrice *big.Int)  *big.Int {
	networkIntPrice := int(networkGasPrice.Uint64())
	MEAN := historicalMeanGasPrice
	var discounted int
	switch  {
	case networkIntPrice <= 5*MEAN/10:
		discounted = 3*MEAN/10
	case networkIntPrice <= 7*MEAN/10:
		discounted = 4*MEAN/10
	case networkIntPrice <= 9*MEAN/10:
		discounted = 5*MEAN/10
	case networkIntPrice <= 12*MEAN/10:
		discounted = 6*MEAN/10
	case networkIntPrice <= 14*MEAN/10:
		discounted = 7*MEAN/10
	case networkIntPrice <= 16*MEAN/10:
		discounted = 8*MEAN/10
	default:
		discounted = MEAN; // gas price never above mean
	}
	// verify calculation does not increase the gas-price (may happen if network_price < MEAN)
	if (discounted > networkIntPrice) {
		return networkGasPrice;
	}
	return big.NewInt(int64(discounted))
}


// check if erc20 transfer tx and, if so, return the function type
func isErc20TransferTx(to *common.Address, data []byte) funcType {
	if !supportWhitelistedErc20s {
		return ftype_none
	}
	if to == nil || data == nil {
		return ftype_none
	}
	// verify 'to' address is of a whitelisted ERC20 contract
	if !isWhitelistedErc20(*to) {
		return ftype_none
	}
	dataLen := len(data)
	functionSelector, err := getFunctionSelector(data, dataLen)
	if err != nil {
		return ftype_none
	}
	// look for transfer(), transferFrom() or approve() calls
	switch {
	case validTransferData(functionSelector, dataLen):
		return ftype_transfer
	case validTransferFromData(functionSelector, dataLen):
		return ftype_transferFrom
	case validApproveData(functionSelector, dataLen):
		return ftype_approve
	}
	log.Debug("@lfm: erc20 non-transfer tx invoked on whitelisted erc20", "functionSelector", functionSelector)
	return ftype_none
}

func isWhitelistedErc20(addr common.Address) bool {
	return internalErc20Whitelist[addr]
}

// apply gas-price discount for whitelisted-erc20 transfer tx
func discountErc20TransferTx(networkGasPrice *big.Int, ftype funcType, erc20TransferGasAmount uint64) *big.Int {
	coreTransferGasPrice := calcCoreTransferDiscountedGasPrice(networkGasPrice).Uint64()
	var erc20TransferGasPrice uint64
	if ftype == ftype_transfer || ftype == ftype_transferFrom {
		// for transfer[From]: total gas cost for whitelisted erc20 should be same as in core transfer
		erc20TransferGasPrice = coreTransferGasPrice * coreTransferGasAmount / erc20TransferGasAmount
		log.Debug("@lfm: erc20 transfer discount", "networkGasPrice", networkGasPrice, "discounted-gas-price", erc20TransferGasPrice)
	} else if ftype == ftype_approve {
		// for approve: gas price (not fee!) should be same as in core transfer
		erc20TransferGasPrice = coreTransferGasPrice
		log.Debug("@lfm: erc20 approve discount", "networkGasPrice", networkGasPrice, "discounted-gas-price", erc20TransferGasPrice)
	} else {
		log.Warn("@lfm: discountErc20TransferTx bad function type", "function-type", ftype)
	}
	return new(big.Int).SetUint64(erc20TransferGasPrice)
}

func getFunctionSelector(data []byte, dataLen int) ([]byte,error) {
	if dataLen < functionSelectorSize {
		log.Warn("@lfm: erc20 error: dataLen < functionSelectorSize", "dataLen", dataLen)
		return nil, errNotFunctionInvocation
	}
	functionSelector := data[:functionSelectorSize]
	return functionSelector, nil
}

func validTransferData(functionSelector []byte, dataLen int) bool {
	if !eq(functionSelector, transferFunction) {
		return false
	}
	if dataLen != transferDataLen {
		log.Warn("@lfm: erc20 transfer() called with bad data", "dataLen", dataLen)
		return false
	}
	log.Debug("@lfm: erc20 transfer() call detected for gas price discount")
	return true
}

func validTransferFromData(functionSelector []byte, dataLen int) bool {
	if !eq(functionSelector, transferFromFunction) {
		return false
	}
	if dataLen != transferFromDataLen {
		log.Warn("@lfm: erc20 transferFrom() called with bad data", "dataLen", dataLen)
		return false
	}
	log.Debug("@lfm: erc20 transferFrom() call detected for gas price discount")
	return true

}

func validApproveData(functionSelector []byte, dataLen int) bool {
	if !eq(functionSelector, approveFunction) {
		return false
	}
	if dataLen != approveDataLen {
		log.Warn("@lfm: erc20 approve() called with bad data", "dataLen", dataLen)
		return false
	}
	log.Debug("@lfm: erc20 approve() call detected for gas price discount")
	return true
}

func eq(a []byte, b []byte) bool {
	return bytes.Equal(a, b)
}
