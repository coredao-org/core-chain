package satoshi

import (
	"context"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/eth/config"
	"github.com/ethereum/go-ethereum/eth/gasprice/lfm"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
)

func (p *Satoshi) refreshNetworkConfigCacheIfNeeded(currentBlockNumber uint64) {
	networkConfigCache := lfm.GetNetworkConfigCache()
	if !timeToRefreshCache(currentBlockNumber, networkConfigCache) {
		return
	}
	meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, ok := p.readConfigParamsFromContract()
	if !ok {
		return
	}
	// and update the node's config cache
	networkConfigCache.UpdateValues(meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, currentBlockNumber)
}

func (p *Satoshi) readConfigParamsFromContract() (int, uint64, []uint64, []uint64, map[common.Address]uint64, bool) {
	// get raw results from config contract
	resultBytes := queryNetworkConfigContract(p.ethAPI, p.networkConfigABI)
	if resultBytes == nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to read contract")
		return configParamsError()
	}
	// unpack and parse results into config values
	meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, ok := p.parseConfigResults(resultBytes)
	if !ok {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to parse config results")
		return configParamsError()
	}
	return meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, true
}

func (p *Satoshi) parseConfigResults(resultBytes hexutil.Bytes) (int, uint64, []uint64, []uint64, map[common.Address]uint64, bool) {
	var unpacked []interface{}
	err := p.networkConfigABI.UnpackIntoInterface(&unpacked, networkConfigGetterFunction, resultBytes)
	if err != nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to invoke networkConfig getter function", "error", err)
		return configParamsError()
	}
	meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, ok := parseUnpackedResults(unpacked)
	if !ok {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to parse getter results")
		return configParamsError()
	}
	return meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, true
}

func queryNetworkConfigContract(ethAPI *ethapi.PublicBlockChainAPI, networkConfigABI abi.ABI) hexutil.Bytes {
	if ethAPI == nil {
		log.Warn("@lfm queryNetworkConfigContract error: no satoshi.EthAPI")
		return nil
	}
	// obtain getter's method signature
	signature, err := networkConfigABI.Pack(networkConfigGetterFunction)
	if err != nil {
		log.Warn("@lfm queryNetworkConfigContract error: could not pack getter function", "getterFunc", networkConfigGetterFunction, "error", err)
		return nil
	}

	msg := ethapi.TransactionArgs{
		To:   &networkConfigContractAddress,
		Data: (*hexutil.Bytes)(&signature),
	}
	latestBlockNrOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	ctx := context.Background()
	// invoke smart-contract getter
	resultBytes, err := ethAPI.Call(ctx, msg, latestBlockNrOrHash, nil)
	if err != nil {
		log.Warn("@lfm queryNetworkConfigContract error: failed to invoke contract function", "error", err)
		return nil
	}
	if resultBytes == nil || len(resultBytes) == 0 {
		log.Warn("@lfm queryNetworkConfigContract returned an empty result")
		return nil
	}
	return resultBytes
}

func timeToRefreshCache(currentBlockNumber uint64, networkConfigCache *config.NetworkConfigCache) bool {
	if networkConfigCache.NotInitialized() {
		return true
	}
	refreshIntervalInBlocks := networkConfigCache.RefreshIntervalInBlocks()
	refreshBlockReached := currentBlockNumber % refreshIntervalInBlocks == 0
	if !refreshBlockReached {
		return false
	}
	lastRefreshBlockNumber := networkConfigCache.LastRefreshBlockNumber()
	if lastRefreshBlockNumber == currentBlockNumber {
		return false // already refreshed
	}
	return true
}

func parseUnpackedResults(unpacked []interface{}) (int, uint64, []uint64, []uint64, map[common.Address]uint64, bool) {
	const configParamCount = 6
	count := len(unpacked)
	if count != configParamCount {
		log.Warn("@lfm parseUnpackedResults - bad returned value count", "count", count)
		return configParamsError()
	}
	meanGasPrice, ok := unpackedToInt(unpacked[0], "meanGasPrice")
	if !ok {
		log.Warn("@lfm parseUnpackedResults - failed to unpack meanGasPrice")
		return configParamsError()
	}
	refreshInterval, ok := unpackedToUInt(unpacked[1], "refreshInterval")
	if !ok {
		log.Warn("parseUnpackedResults - failed to unpack refreshInterval")
		return configParamsError()
	}
	gasPriceSteps, ok := unpackedToUIntArray(unpacked[2], "gasPriceSteps")
	if !ok {
		log.Warn("parseUnpackedResults - failed to unpack gasPriceSteps")
		return configParamsError()
	}
	gasDiscountedPrices, ok := unpackedToUIntArray(unpacked[3], "gasDiscountedPrices")
	if !ok {
		log.Warn("parseUnpackedResults - failed to unpack gasDiscountedPrices")
		return configParamsError()
	}
	if len(gasPriceSteps) != len(gasDiscountedPrices) {
		log.Warn("parseUnpackedResults - gas price steps and discounted lengths for native CORE transfer don't match")
		return configParamsError()
	}
	destinationAddresses, ok := unpacked[4].([]common.Address)
	if !ok {
		log.Warn("parseUnpackedResults - failed to unpack destinationAddresses")
		return configParamsError()
	}
	destinationGasFactors, ok := unpackedToUIntArray(unpacked[5], "destinationGasFactors")
	if !ok {
		log.Warn("parseUnpackedResults - failed to unpack destinationGasFactors")
		return configParamsError()
	}
	if len(destinationAddresses) != len(destinationGasFactors) {
		log.Warn("parseUnpackedResults - destination addresses and gasFactor lengths don't match")
		return configParamsError()
	}
	// params are fine
	destinationGasFactorMap := createDestinationGasFactorMap(destinationAddresses, destinationGasFactors)
	return meanGasPrice, refreshInterval, gasPriceSteps, gasDiscountedPrices, destinationGasFactorMap, true
}

func unpackedToUIntArray(unpacked interface{}, name string) ([]uint64, bool) {
	gasPriceStepsBig, ok := unpacked.([]*big.Int)
	if !ok {
		log.Warn("unpackedToUIntArray - result not BigInt array", "name", name)
		return nil, false
	}
	gasPriceSteps, ok := convertBigToUIntArray(gasPriceStepsBig)
	if !ok {
		log.Warn("unpackedToUIntArray - failed to cast array to uint array","name", name)
		return nil, false
	}
	return gasPriceSteps, true
}

func unpackedToInt(unpacked interface{}, name string) (int, bool) {
	bigInt, ok := unpacked.(*big.Int)
	if !ok {
		log.Warn("@lfm unpackedToInt - bad parameter type - expected BigInt", "name", name)
		return 0, false
	}
	if !bigInt.IsInt64() {
		log.Warn("@lfm unpackedToInt - not a valid int64 value", "name", name)
		return 0, false
	}
	return int(bigInt.Int64()), true
}

func unpackedToUInt(unpacked interface{}, name string) (uint64, bool) {
	bigInt, ok := unpacked.(*big.Int)
	if !ok {
		log.Warn("@lfm unpackedToUInt - bad parameter type - expected BigInt", "name", name)
		return 0, false
	}
	if !bigInt.IsUint64() {
		log.Warn("@lfm unpackedToUInt - not a valid uint64 value", "name", name)
		return 0, false
	}
	return bigInt.Uint64(), true
}

func createDestinationGasFactorMap(addresses []common.Address, factors []uint64) map[common.Address]uint64 {
	gasFactors := make(map[common.Address]uint64)
	for i := 0; i < len(addresses); i++ {
		gasFactors[addresses[i]] = factors[i]
	}
	return gasFactors
}

func convertBigToUIntArray(bigIntArr []*big.Int) ([]uint64, bool) {
	uint64Slice := make([]uint64, len(bigIntArr))
	for i, bigInt := range bigIntArr {
		if !bigInt.IsUint64() {
			log.Warn("value at index %d is too large for uint64", "index", i, "value", bigInt)
			return nil, false
		}
		uint64Slice[i] = bigInt.Uint64()
	}
	return uint64Slice, true
}

func configParamsError() (int, uint64, []uint64, []uint64, map[common.Address]uint64, bool) {
	return 0, 0, nil, nil, nil, false
}

