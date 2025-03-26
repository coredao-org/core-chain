package feemarket

import (
	"math"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const (
	DENOMINATOR_STORAGE_SLOT             = 1
	MAX_REWARDS_STORAGE_SLOT             = 2
	MAX_EVENTS_STORAGE_SLOT              = 3
	MAX_FUNCTION_SIGNATURES_STORAGE_SLOT = 4
	MAX_GAS_STORAGE_SLOT                 = 5
	CONFIGS_STORAGE_SLOT                 = 6
)

// StorageProvider implements the Provider interface using direct storage access
type StorageProvider struct {
	// contractAddr is the address of the fee market contract
	contractAddr common.Address

	// configCache is a cache of configs by address
	configCache map[common.Address]types.FeeMarketConfig

	denominator           atomic.Uint64
	maxRewards            atomic.Uint64
	maxGas                atomic.Uint64
	maxEvents             atomic.Uint64
	maxFunctionSignatures atomic.Uint64

	lock sync.RWMutex // Protects the cache access
}

// NewStorageProvider creates a new provider that reads directly from storage
func NewStorageProvider(contractAddr common.Address) (*StorageProvider, error) {
	provider := &StorageProvider{
		contractAddr: contractAddr,
		configCache:  make(map[common.Address]types.FeeMarketConfig),
	}

	// TODO: shall we read the DENOMINATOR from the contract?

	// does it make sense to warm up all configs from storage?
	// p.reloadAllConfigs(state)

	return provider, nil
}

// GetDenominator reads the denominator from storage
func (p *StorageProvider) GetDenominator(state FeeMarketStateReader) uint64 {
	cachedDenominator := p.denominator.Load()
	if cachedDenominator != 0 {
		return cachedDenominator
	}

	denominatorSlot := common.BigToHash(big.NewInt(DENOMINATOR_STORAGE_SLOT))
	denominatorBytes := state.GetState(p.contractAddr, denominatorSlot)
	denominator := new(uint256.Int).SetBytes(denominatorBytes[:]).Uint64()
	p.denominator.Store(denominator)
	return denominator
}

// GetMaxRewards reads the max rewards from storage
func (p *StorageProvider) GetMaxRewards(state FeeMarketStateReader) uint64 {
	cachedMaxRewards := p.maxRewards.Load()
	if cachedMaxRewards != 0 {
		return cachedMaxRewards
	}

	maxRewardsSlot := common.BigToHash(big.NewInt(MAX_REWARDS_STORAGE_SLOT))
	maxRewardsBytes := state.GetState(p.contractAddr, maxRewardsSlot)
	maxRewards := new(uint256.Int).SetBytes(maxRewardsBytes[:]).Uint64()
	p.maxRewards.Store(maxRewards)
	return maxRewards
}

// GetMaxGas reads the max gas from storage
func (p *StorageProvider) GetMaxGas(state FeeMarketStateReader) uint64 {
	cachedMaxGas := p.maxGas.Load()
	if cachedMaxGas != 0 {
		return cachedMaxGas
	}

	maxGasSlot := common.BigToHash(big.NewInt(MAX_GAS_STORAGE_SLOT))
	maxGasBytes := state.GetState(p.contractAddr, maxGasSlot)
	maxGas := new(uint256.Int).SetBytes(maxGasBytes[:]).Uint64()
	p.maxGas.Store(maxGas)
	return maxGas
}

// GetMaxEvents reads the max events from storage
func (p *StorageProvider) GetMaxEvents(state FeeMarketStateReader) uint64 {
	cachedMaxEvents := p.maxEvents.Load()
	if cachedMaxEvents != 0 {
		return cachedMaxEvents
	}

	maxEventsSlot := common.BigToHash(big.NewInt(MAX_EVENTS_STORAGE_SLOT))
	maxEventsBytes := state.GetState(p.contractAddr, maxEventsSlot)
	maxEvents := new(uint256.Int).SetBytes(maxEventsBytes[:]).Uint64()
	if maxEvents > math.MaxUint8 {
		maxEvents = math.MaxUint8
		// or should we panic?
	}
	p.maxEvents.Store(maxEvents)
	return maxEvents
}

// GetMaxFunctionSignatures reads the max function signatures from storage
func (p *StorageProvider) GetMaxFunctionSignatures(state FeeMarketStateReader) uint64 {
	cachedMaxFunctionSignatures := p.maxFunctionSignatures.Load()
	if cachedMaxFunctionSignatures != 0 {
		return cachedMaxFunctionSignatures
	}

	maxFunctionSignaturesSlot := common.BigToHash(big.NewInt(MAX_FUNCTION_SIGNATURES_STORAGE_SLOT))
	maxFunctionSignaturesBytes := state.GetState(p.contractAddr, maxFunctionSignaturesSlot)
	maxFunctionSignatures := new(uint256.Int).SetBytes(maxFunctionSignaturesBytes[:]).Uint64()
	p.maxFunctionSignatures.Store(maxFunctionSignatures)
	return maxFunctionSignatures
}

// InvalidateConstants invalidates the constants in the cache
func (p *StorageProvider) InvalidateConstants() {
	p.denominator.Store(0)
	p.maxRewards.Store(0)
	p.maxGas.Store(0)
	p.maxEvents.Store(0)
	p.maxFunctionSignatures.Store(0)
}

// GetConfig returns a config for an address, with caching
func (p *StorageProvider) GetConfig(address common.Address, state FeeMarketStateReader) (types.FeeMarketConfig, bool) {
	// Load config from cache, if found
	p.lock.RLock()
	config, found := p.configCache[address]
	if found {
		p.lock.RUnlock()
		return config, true
	}
	p.lock.RUnlock()

	// all := readAllConfigAddresses(state)
	// if address in all{
	// }

	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	config, found = p.findConfigForAddress(address, state)
	if !config.IsValidConfig(p.GetDenominator(state), p.GetMaxGas(state), p.GetMaxEvents(state), p.GetMaxRewards(state)) {
		return types.FeeMarketConfig{}, false
	}

	if found {
		p.lock.Lock()
		p.configCache[address] = config // set in cache
		p.lock.Unlock()
	}

	return config, found
}

// InvalidateConfig removes a specific address from the cache
func (p *StorageProvider) InvalidateConfig(address common.Address) {
	p.lock.Lock()
	delete(p.configCache, address)
	p.lock.Unlock()
}

// CleanCache cleans the cache
func (p *StorageProvider) CleanCache() {
	p.lock.Lock()
	p.configCache = make(map[common.Address]types.FeeMarketConfig)
	p.lock.Unlock()
}

// loadAllConfigs reloads all configs from storage (must be called with lock held)
func (p *StorageProvider) loadAllConfigs(state FeeMarketStateReader) {
	if state == nil {
		return
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	// Clear the cache
	p.configCache = make(map[common.Address]types.FeeMarketConfig)

	configsLength, err := p.readConfigsLength(state)
	if err != nil {
		log.Error("Failed to read configs length", "err", err)
		return
	}

	// Read each config
	for i := uint64(0); i < configsLength; i++ {
		config, err := p.readConfigAtIndex(i, state)
		if err != nil {
			log.Error("Failed to read config", "index", i, "err", err)
			continue
		}

		if !config.IsValidConfig(p.GetDenominator(state), p.GetMaxGas(state), p.GetMaxEvents(state), p.GetMaxRewards(state)) {
			log.Info("Invalid fee market config in storage", "address", config.ConfigAddress)
			continue
		}

		p.configCache[config.ConfigAddress] = config
	}
}

// findConfigForAddress finds a config for an address by scanning all configs
func (p *StorageProvider) findConfigForAddress(address common.Address, state FeeMarketStateReader) (types.FeeMarketConfig, bool) {
	configsLength, err := p.readConfigsLength(state)
	if err != nil {
		log.Error("Failed to read configs length", "err", err)
		return types.FeeMarketConfig{}, false
	}

	// Scan all configs to find one with matching address
	for i := uint64(0); i < configsLength; i++ {
		config, err := p.readConfigAtIndex(i, state)
		if err != nil {
			log.Debug("Failed to read config for FeeMarket configuration", "index", i, "err", err)
			continue
		}

		if config.ConfigAddress == address {
			return config, true
		}

		// TODO: we can check if config is valid here and add it to the cache?
	}

	return types.FeeMarketConfig{}, false
}

// readConfigsLength reads the length of the configs array from storage
func (p *StorageProvider) readConfigsLength(state FeeMarketStateReader) (uint64, error) {
	// In Solidity, the length of a public array is stored at the array's slot
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	lengthBytes := state.GetState(p.contractAddr, configsSlot)
	return new(uint256.Int).SetBytes(lengthBytes[:]).Uint64(), nil
}

// readConfigAtIndex reads a config from storage at a specific index
func (p *StorageProvider) readConfigAtIndex(index uint64, state FeeMarketStateReader) (config types.FeeMarketConfig, err error) {
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))

	// Calculate base slot for configs array
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// Each Config takes 3 slots (packed fields, events.length, functionSignatures.length)
	configSizeInSlots := uint64(3)

	// Calculate this config's starting slot
	indexOffset := new(big.Int).Mul(
		big.NewInt(int64(configSizeInSlots)),
		big.NewInt(int64(index)),
	)
	configSlot := common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(configsBaseSlot[:]),
		indexOffset,
	))

	// Read packed isActive and configAddress (at slot+2)
	// Solidity packs bool (1 byte) and address (20 bytes) into a single slot
	packedSlot := configSlot
	packedData := state.GetState(p.contractAddr, packedSlot)

	// Extract isActive (lowest byte, rightmost bit)
	isActive := packedData[11]&0x01 == 1
	configAddr := common.BytesToAddress(packedData[12:32])

	// short circuit if config is already in cache, so as to avoid reading from storage
	p.lock.RLock()
	config, found := p.configCache[configAddr]
	p.lock.RUnlock()
	if found {
		return config, nil
	}

	// Read events array length (right-aligned)
	eventsLengthSlot := incrementHash(packedSlot)
	eventsLengthData := state.GetState(p.contractAddr, eventsLengthSlot)
	eventsLength := new(uint256.Int).SetBytes(eventsLengthData.Bytes()).Uint64()

	// Read events
	eventsBaseSlot := common.BytesToHash(crypto.Keccak256(eventsLengthSlot[:]))
	events := make([]types.FeeMarketEvent, eventsLength)
	for i := uint64(0); i < eventsLength; i++ {
		// Each Event takes 3 slots (eventSignature, gas, rewards.length)
		eventSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(eventsBaseSlot.Bytes()),
			new(big.Int).Mul(big.NewInt(3), big.NewInt(int64(i))),
		))

		eventSigSlot := eventSlot
		gasSlot := incrementHash(eventSigSlot)
		rewardsLengthSlot := incrementHash(gasSlot)

		// Read rewards
		rewardsLength := new(uint256.Int).SetBytes(
			state.GetState(p.contractAddr, rewardsLengthSlot).Bytes(),
		).Uint64()
		rewards := readRewards(p.contractAddr, rewardsLengthSlot, rewardsLength, state)

		events[i] = types.FeeMarketEvent{
			EventSignature: state.GetState(p.contractAddr, eventSigSlot),
			Gas:            new(uint256.Int).SetBytes(state.GetState(p.contractAddr, gasSlot).Bytes()).Uint64(),
			Rewards:        rewards,
		}
	}

	// Function signatures will be handled later

	config = types.FeeMarketConfig{
		ConfigAddress: configAddr,
		IsActive:      isActive,
		Events:        events,
	}

	return config, nil
}

// Helper function to read rewards array
func readRewards(contractAddr common.Address, rewardsLengthSlot common.Hash, rewardsLength uint64, state FeeMarketStateReader) []types.FeeMarketReward {
	rewardsBaseSlot := common.BytesToHash(crypto.Keccak256(rewardsLengthSlot[:]))
	rewards := make([]types.FeeMarketReward, rewardsLength)

	for i := uint64(0); i < rewardsLength; i++ {
		// Each Reward takes 1 slot with packed fields (rewardAddr, rewardPercentage)
		rewardSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(rewardsBaseSlot[:]),
			big.NewInt(int64(i)),
		))

		// First 20 bytes for the address
		// Last 2 bytes for the percentage (uint16)
		// Solidity packs data right-aligned, so we need to read from the end
		packedBytes := state.GetState(contractAddr, rewardSlot)
		rewardAddress := common.BytesToAddress(packedBytes[12:32])
		rewardPercentage := new(uint256.Int).SetBytes(packedBytes[10:12]).Uint64()

		rewards[i] = types.FeeMarketReward{
			RewardAddress:    rewardAddress,
			RewardPercentage: rewardPercentage,
		}
	}
	return rewards
}

// incrementHash increments a hash value by 1
func incrementHash(h common.Hash) common.Hash {
	return common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(h[:]),
		big.NewInt(1),
	))
}
