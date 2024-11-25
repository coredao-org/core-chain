package config

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"math/big"
	"strconv"
	"strings"
	"sync"
)

const initialMeanGasPrice = 35783571428 // hard-coded gas amount

var thousand = big.NewInt(1000)

type NetworkConfigCache struct {
	mu sync.RWMutex
	initialized bool
	meanGasPrice int
	lastRefreshBlockNumber uint64
	refreshIntervalInBlocks uint64 // cache refresh interval in blocks
	gasPriceSteps []uint64
	gasDiscountedPrices    []uint64
	destinationGasFactorsMillis map[common.Address]uint64
}

func DefaultNetworkConfigCache() *NetworkConfigCache {
	return &NetworkConfigCache{
		initialized:             false,
		meanGasPrice:            initialMeanGasPrice, //wei
		gasPriceSteps:           []uint64{},
		gasDiscountedPrices:     []uint64{},
		lastRefreshBlockNumber:  0,
		refreshIntervalInBlocks: 1, // refresh every block
		destinationGasFactorsMillis: make(map[common.Address]uint64),
	}
}

func (cache *NetworkConfigCache) UpdateValues(meanGasPrice int, refreshIntervalInBlocks uint64,
											  gasPriceSteps []uint64, gasDiscountedPrices []uint64,
											  destinationGasFactors map[common.Address]uint64, blockNumber uint64) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.initialized = true
	cache.meanGasPrice = meanGasPrice
	cache.lastRefreshBlockNumber = blockNumber
	cache.refreshIntervalInBlocks = refreshIntervalInBlocks
	cache.gasPriceSteps = gasPriceSteps
	cache.gasDiscountedPrices = gasDiscountedPrices
	cache.destinationGasFactorsMillis = destinationGasFactors

	log.Debug("@lfm: UpdateValues", "meanGasPrice", cache.meanGasPrice,
					"lastRefreshBlockNumber", cache.lastRefreshBlockNumber, "refreshIntervalInBlocks", refreshIntervalInBlocks,
					"gasPriceSteps", cache.gasPriceSteps, "gasDiscountedPrices", cache.gasDiscountedPrices,
					"destinationGasFactors", joinAddrMap(cache.destinationGasFactorsMillis))
}

func (cache *NetworkConfigCache) AdjustGasPriceForDestination(networkGasPrice *big.Int, destination *common.Address) (*big.Int, bool) {
	if destination == nil {
		return nil, false
	}
	if gasPriceFactor, ok := cache.getGasPriceFactor(*destination); ok {
		return applyGasPriceFactor(gasPriceFactor, networkGasPrice), true
	}
	return nil, false
}

func (cache *NetworkConfigCache) getGasPriceFactor(destination common.Address) (uint64, bool) {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	gasPriceFactor, exist := cache.destinationGasFactorsMillis[destination]
	return gasPriceFactor, exist
}

func applyGasPriceFactor(gasPriceFactorMillis uint64, networkGasPrice *big.Int) *big.Int {
	// adjusted = networkGasPrice * factorInMillis / 1000
	mul := new(big.Int).Mul(networkGasPrice, big.NewInt(int64(gasPriceFactorMillis)))
	adjustedGasPrice := new(big.Int).Div(mul, thousand)
	return adjustedGasPrice
}

func (cache *NetworkConfigCache) HistoricalMeanGasPrice() int {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.meanGasPrice
}

func (cache *NetworkConfigCache) LastRefreshBlockNumber() uint64 {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.lastRefreshBlockNumber
}

func (cache *NetworkConfigCache) RefreshIntervalInBlocks() uint64 {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.refreshIntervalInBlocks
}

func (cache *NetworkConfigCache) DynamicDiscountedGasPrice(networkGasPrice *big.Int) *big.Int {
	currentGasPrice := networkGasPrice.Uint64()
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	// steps/prices both ascending and small (hence no need for bsearch)
	steps := cache.gasPriceSteps
	prices := cache.gasDiscountedPrices
	numSteps := len(steps)
	for i := 0; i < numSteps; i++ {
		if currentGasPrice <= steps[i] {
			discounted := prices[i];
			return new(big.Int).SetUint64(discounted)
		}
	}
	return networkGasPrice
}

func (cache *NetworkConfigCache) NotInitialized() bool {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return !cache.initialized
}

func joinAddrMap(addresses map[common.Address]uint64) string {
	addrArr := make([]string, len(addresses))
	i := 0
	for addr, factor := range addresses {
		factorStr := strconv.FormatUint(factor, 10)
		addrArr[i] = addr.Hex() + ":" + factorStr
		i++
	}
	str := strings.Join(addrArr, ",")
	return str
}
