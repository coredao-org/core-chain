package lfm

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/eth/config"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
)

const (
	useDebugDiscountedGasPrice = false
	useDynamicGasDiscount = false
	coreTransferGasAmount = 21000 // hard-coded gas amount
)

var (
	debugDiscountedGasPrice = new(big.Int).SetUint64(7_000_000_000)
	networkConfigCache = config.DefaultNetworkConfigCache()
)

func AdjustGasPriceForEstimation(networkGasPriceParam *hexutil.Big, gasAmountParam *hexutil.Uint64, valueParam *hexutil.Big, to *common.Address, data []byte) *hexutil.Big {
	if networkGasPriceParam == nil || gasAmountParam == nil || valueParam == nil {
		return networkGasPriceParam
	}
	networkGasPrice := (*big.Int)(networkGasPriceParam)
	gasAmount := uint64(*gasAmountParam)
	value := (*big.Int)(valueParam)
	adjusted := AdjustGasPrice(gasAmount, value, to, data, networkGasPrice)
	return (*hexutil.Big)(adjusted)
}

func GetNetworkConfigCache() *config.NetworkConfigCache {
	return networkConfigCache
}

func AdjustGasPrice(gasAmount uint64, value *big.Int, to *common.Address, data []byte, networkGasPrice *big.Int) *big.Int {
	if networkConfigCache.NotInitialized() {
		return networkGasPrice // configuration missing - fallback to network price
	}
	if adjusted, ok := adjustForCoreTransfer(networkGasPrice, gasAmount, value, len(data)); ok {
		return adjusted
	}
	if adjusted, ok := adjustForWhitelistedDestination(networkGasPrice, to); ok {
		return adjusted
	}
	return networkGasPrice
}

func adjustForCoreTransfer(networkGasPrice *big.Int, gasAmount uint64, value *big.Int, dataLen int) (*big.Int, bool) {
	if isCoreTransfer(gasAmount, value, dataLen) {
		return discountedCoreTransferGasPrice(networkGasPrice), true
	}
	return nil, false
}

func adjustForWhitelistedDestination(networkGasPrice *big.Int, to *common.Address) (*big.Int, bool) {
	return networkConfigCache.AdjustGasPriceForDestination(networkGasPrice, to)
}

func isCoreTransfer(gasAmount uint64, value *big.Int, dataLen int) bool {
	// requires (a) EOA destination (b) value > 0 (c) empty data indicating not a smart-contract
	// invocation and (d) total gasAmount == 21k so to rule out transfer into a smart contract (and not EOA)
	hasValue := value != nil && value.Sign() > 0
	eoaToAddress := gasAmount == coreTransferGasAmount
	isCoreTransferTx := hasValue && eoaToAddress && dataLen == 0
	return isCoreTransferTx
}

func discountedCoreTransferGasPrice(networkGasPrice *big.Int) *big.Int {
	// apply gas-price discount for CORE transfer tx
	if useDebugDiscountedGasPrice {
		log.Debug("@lfm: debug-mode CORE transfer discount", "networkGasPrice", networkGasPrice, "debugGasPrice", debugDiscountedGasPrice)
		return debugDiscountedGasPrice
	}
	discountedGasPrice := calcCoreTransferDiscountedGasPrice(networkGasPrice)
	log.Debug("@lfm: CORE transfer discount", "networkGasPrice", networkGasPrice, "discounted-gas-price", discountedGasPrice)
	return discountedGasPrice
}

func calcCoreTransferDiscountedGasPrice(networkGasPrice *big.Int)  *big.Int {
	if useDynamicGasDiscount {
		return networkConfigCache.DynamicDiscountedGasPrice(networkGasPrice)
	} else {
		return getDiscountedGasPrice(networkGasPrice)
	}
}

/**
  discrete steps for the sigmoid function sig(x) = 1 / (1 + e^(-3 * (x - 0.8)))
  where x denotes the historical mean, slightly modified for simplicity
  using integer steps so to avoid potential floating point inconsistencies between nodes
 */
func getDiscountedGasPrice(networkGasPrice *big.Int)  *big.Int {
	networkIntPrice := int(networkGasPrice.Uint64())
	MEAN := networkConfigCache.HistoricalMeanGasPrice()
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
	// verify calculation does not increase the gas-price (may happen if network_price much below MEAN)
	if discounted > networkIntPrice {
		return networkGasPrice;
	}
	return big.NewInt(int64(discounted))
}
