package satoshi

import (
	"context"
	"fmt"
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
	resultBytes := queryNetworkConfigContract(p.ethAPI, p.networkConfigABI)
	if resultBytes == nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to read contract")
		return
	}
	// Unpack the result
	var unpacked []interface{}
	err := p.networkConfigABI.UnpackIntoInterface(&unpacked, networkConfigGetterFunction, resultBytes)
	if err != nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to invoke networkConfig getter function", "error", err)
		return
	}
	meanGasPrice, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors, err := parseUnpackedResults(unpacked)
	if err != nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to parse getter results", "error", err)
		return
	}
	networkConfigCache.UpdateValues(meanGasPrice, currentBlockNumber, refreshIntervalInBlocks, gasPriceSteps, gasDiscountedPrices, destinationGasFactors)
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


func parseUnpackedResults(unpacked []interface{}) (int, uint64, []uint64, []uint64, map[common.Address]uint64, error) {
	// type-assert and assign the unpacked values
	meanGasPrice := int(unpacked[0].(*big.Int).Int64())
	refreshInterval := unpacked[1].(*big.Int).Uint64()
	gasPriceStepsBig := unpacked[2].([]*big.Int)
	gasDiscountedPricesBig := unpacked[3].([]*big.Int)
	destinationAddresses := unpacked[4].([]common.Address)
	destinationGasFactors := unpacked[5].([]uint64)

	gasPriceSteps, err := toUint64Arr(gasPriceStepsBig)
	if err != nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to parse gasPriceSteps array", "error", err)
		return 0, 0, nil, nil, nil, err
	}
	gasDiscountedPrices, err := toUint64Arr(gasDiscountedPricesBig)
	if err != nil {
		log.Warn("@lfm LoadNetworkConfigIfNeeded - failed to parse discountedPrices array", "error", err)
		return 0, 0, nil, nil, nil, err
	}
	destinationGasFactorMap := createDestinationGasFactorMap(destinationAddresses, destinationGasFactors)
	return meanGasPrice, refreshInterval, gasPriceSteps, gasDiscountedPrices, destinationGasFactorMap, nil
}

func createDestinationGasFactorMap(addresses []common.Address, factors []uint64) map[common.Address]uint64 {
	gasFactors := make(map[common.Address]uint64)
	for i := 0; i < len(addresses); i++ {
		gasFactors[addresses[i]] = factors[i]
	}
	return gasFactors
}

func toUint64Arr(bigInts []*big.Int) ([]uint64, error) {
	uint64Slice := make([]uint64, len(bigInts))
	for i, bint := range bigInts {
		if !bint.IsUint64() {
			return nil, fmt.Errorf("value at index %d is too large for uint64", i)
		}
		uint64Slice[i] = bint.Uint64()
	}
	return uint64Slice, nil
}

