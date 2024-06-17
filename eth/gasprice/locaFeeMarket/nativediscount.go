package locaFeeMarket

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
)

const (
	/*
		Sigmoid function: Y = Y0 + L/(1 + e^(-k(x - x0)))
		where:
			L  = historicalSDGasPrice/2
			Y0 = historicalMeanGasPrice
			x0 = historicalMeanGasPrice
			k  = 1e−10 (small value to control the steepness)

		behavior:
			- y=x for {x=historicalMean}
			- y never exceeds maxAdjusted value
			- smooth 'logarithmic' slope
	*/
	historicalMeanGasPrice = 35783571428 //wei
	historicalSDGasPrice = 849870638     //wei
	maxAdjusted = historicalMeanGasPrice + historicalSDGasPrice/2 // =36208506747wei
)

const (
	nativeTransferTxGas = 21000
	lfmDebugMode        = true //@hhhh
)

func AdjustGasPriceForEstimation(_origGasPrice *hexutil.Big, _gas *hexutil.Uint64, _value *hexutil.Big, dataLen int) *hexutil.Big {
	if _origGasPrice == nil || _gas == nil || _value == nil {
		return _origGasPrice
	}
	origGasPrice := (*big.Int)(_origGasPrice)
	gas := uint64(*_gas)
	value := (*big.Int)(_value)
	adjusted := origGasPrice
	if isNativeTransferTx(gas, value, dataLen) {
		adjusted = adjustGasPrice(origGasPrice)
	}
	return (*hexutil.Big)(adjusted)
}

func AdjustGasPrice(origGasPrice *big.Int, gas uint64, value *big.Int, dataLen int) *big.Int { //@lfm
	if isNativeTransferTx(gas, value, dataLen) {
		return adjustGasPrice(origGasPrice)
	}
	return origGasPrice
}

func isNativeTransferTx(gas uint64, value *big.Int, dataLen int) bool {
	// adjust gas price of native token transfer txs, and only for EOA destination
	// native transfers to contracts may reesult in gas amounts way above 21k depending on the complexity of the contract's receive()
	hasValue := value != nil && value.Sign() > 0
	toEOA := gas == nativeTransferTxGas
	return hasValue && toEOA && dataLen == 0
}

func adjustGasPrice(origGasPrice *big.Int) *big.Int {
	log.Debug("adjustGasPrice...")
	if lfmDebugMode {
		return new(big.Int).SetUint64(18000000000)
	}

	// using integer steps so to avoid potential floating point inconsistencies between nodes
	orig := origGasPrice.Uint64()
	const MEAN = historicalMeanGasPrice
	var adjusted uint64
	switch {
	case orig <= MEAN:
		adjusted = orig // no adjustment
	case orig <= MEAN+789473684:
		adjusted = MEAN + 220850187
	case orig <= MEAN+1578947368:
		adjusted = MEAN + 229206660
	case orig <= MEAN+2368421053:
		adjusted = MEAN + 237511346
	case orig <= MEAN+3157894737:
		adjusted = MEAN + 245739149
	case orig <= MEAN+3947368421:
		adjusted = MEAN + 253865910
	case orig <= MEAN+4736842105:
		adjusted = MEAN + 261868680
	case orig <= MEAN+5526315789:
		adjusted = MEAN + 269725961
	case orig <= MEAN+6315789474:
		adjusted = MEAN + 277417917
	case orig <= MEAN+7105263158:
		adjusted = MEAN + 284926545
	case orig <= MEAN+7894736842:
		adjusted = MEAN + 292235799
	case orig <= MEAN+8684210526:
		adjusted = MEAN + 299331682
	case orig <= MEAN+9473684211:
		adjusted = MEAN + 306202288
	case orig <= MEAN+10263157895:
		adjusted = MEAN + 312837812
	case orig <= MEAN+11052631579:
		adjusted = MEAN + 319230518
	case orig <= MEAN+11842105263:
		adjusted = MEAN + 325374683
	case orig <= MEAN+12631578947:
		adjusted = MEAN + 331266505
	case orig <= MEAN+13421052632:
		adjusted = MEAN + 336903992
	case orig <= MEAN+14210526316:
		adjusted = MEAN + 342286837
	default:
		adjusted = maxAdjusted
	}

	if adjusted != orig {
		adjusted = min(adjusted, orig)        // sanity check: adjusted gas-price cannot exceed orig
		adjusted = min(adjusted, maxAdjusted) // sanity check: adjusted gas-price cannot exceed maxAdjusted value
		adjusted = max(adjusted, orig/2)      // sanity check: adjusted gas-price cannot go below half of the orig price
	}
	return new(big.Int).SetUint64(adjusted)
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
