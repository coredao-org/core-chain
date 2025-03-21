package feemarket

import (
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const (
	CONFIGS_STORAGE_SLOT_IN_CONTRACT = 2
)

// StorageProvider implements the Provider interface using direct storage access
type StorageProvider struct {
	// contractAddr is the address of the fee market contract
	contractAddr common.Address

	// configCache is a cache of configs by address
	configCache map[common.Address]types.FeeMarketConfig

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

	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	// TODO: we can also lock only when setting in cache
	p.lock.Lock()
	defer p.lock.Unlock()

	// Double-check cache after acquiring exclusive lock
	config, found = p.configCache[address]
	if found {
		return config, true
	}

	config, found = p.findConfigForAddress(address, state)
	if !config.IsValidConfig(GetDenominator()) {
		return types.FeeMarketConfig{}, false
	}

	if found {
		p.configCache[address] = config // set in cache
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

// reloadAllConfigs reloads all configs from storage (must be called with lock held)
func (p *StorageProvider) reloadAllConfigs(state FeeMarketStateReader) {
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

		if !config.IsValidConfig(GetDenominator()) {
			log.Info("Invalid fee market config in storage", "address", config.ConfigAddress)
			continue
		}

		p.configCache[config.ConfigAddress] = config
	}
}

// findConfigForAddress finds a config for an address by scanning all configs
func (p *StorageProvider) findConfigForAddress(address common.Address, state FeeMarketStateReader) (types.FeeMarketConfig, bool) {
	configsLength, err := p.readConfigsLength(state)
	// fmt.Println("- findConfigForAddress -> configsLength:", configsLength)
	if err != nil {
		log.Error("Failed to read configs length", "err", err)
		return types.FeeMarketConfig{}, false
	}

	// Scan all configs to find one with matching address
	for i := uint64(0); i < configsLength; i++ {
		config, err := p.readConfigAtIndex(i, state)
		if err != nil {
			log.Error("Failed to read config", "index", i, "err", err)
			continue
		}

		if config.ConfigAddress == address {
			return config, true
		}
	}

	return types.FeeMarketConfig{}, false
}

// readConfigsLength reads the length of the configs array from storage
func (p *StorageProvider) readConfigsLength(state FeeMarketStateReader) (uint64, error) {
	// In Solidity, the length of a public array is stored at the array's slot
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT_IN_CONTRACT))
	lengthBytes := state.GetState(p.contractAddr, configsSlot)
	return new(uint256.Int).SetBytes(lengthBytes[:]).Uint64(), nil
}

// readConfigAtIndex reads a config from storage at a specific index
func (p *StorageProvider) readConfigAtIndex(index uint64, state FeeMarketStateReader) (types.FeeMarketConfig, error) {
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT_IN_CONTRACT))

	// Calculate base slot for configs array
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// Each Config takes 3 slots (events.length, functionSignatures.length, packed fields)
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

	// First slot is holding the events and functionSignatures lengths (slot + 0)
	// Read events array length (right-aligned)
	packedArraysLengthSlot := configSlot
	packedArraysLengthData := state.GetState(p.contractAddr, packedArraysLengthSlot)
	eventsLength := new(uint256.Int).SetBytes(packedArraysLengthData[16:32]).Uint64()
	functionSignaturesLength := new(uint256.Int).SetBytes(packedArraysLengthData[0:15]).Uint64()

	// Read events
	eventsBaseSlot := common.BytesToHash(crypto.Keccak256(packedArraysLengthSlot[:]))
	events := make([]types.FeeMarketEvent, eventsLength)
	for i := uint64(0); i < eventsLength; i++ {
		// Each Event takes 3 slots (rewards.length, eventSignature, gas)
		eventSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(eventsBaseSlot.Bytes()),
			new(big.Int).Mul(big.NewInt(3), big.NewInt(int64(i))),
		))

		// Read rewards
		rewardsLength := new(uint256.Int).SetBytes(
			state.GetState(p.contractAddr, eventSlot).Bytes(),
		).Uint64()
		rewards := readRewards(p.contractAddr, eventSlot, rewardsLength, state)

		// Read eventSignature and gas
		eventSigSlot := incrementHash(eventSlot)
		gasSlot := incrementHash(eventSigSlot)

		events[i] = types.FeeMarketEvent{
			Rewards:        rewards,
			EventSignature: state.GetState(p.contractAddr, eventSigSlot),
			Gas:            new(uint256.Int).SetBytes(state.GetState(p.contractAddr, gasSlot).Bytes()),
		}
	}

	// Read functionSignatures
	functionSignaturesBaseSlot := incrementHash(packedArraysLengthSlot)
	functionSignatures := make([]types.FeeMarketFunctionSignature, functionSignaturesLength)
	for i := uint64(0); i < functionSignaturesLength; i++ {
		// Each Event takes 3 slots (rewards.length, funcSigsignature, gas)
		functionSignatureslot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(functionSignaturesBaseSlot.Bytes()),
			new(big.Int).Mul(big.NewInt(3), big.NewInt(int64(i))),
		))

		// Read rewards
		rewardsLength := new(uint256.Int).SetBytes(
			state.GetState(p.contractAddr, functionSignatureslot).Bytes(),
		).Uint64()
		rewards := readRewards(p.contractAddr, functionSignatureslot, rewardsLength, state)

		// Read funcSigsignature and gas
		functionSigSlot := incrementHash(functionSignatureslot)
		gasSlot := incrementHash(functionSigSlot)

		functionSignatures[i] = types.FeeMarketFunctionSignature{
			Rewards:           rewards,
			FunctionSignature: state.GetState(p.contractAddr, functionSigSlot),
			Gas:               new(uint256.Int).SetBytes(state.GetState(p.contractAddr, gasSlot).Bytes()),
		}

	}

	// Read packed isActive and configAddress (at slot+2)
	// Solidity packs bool (1 byte) and address (20 bytes) into a single slot
	packedSlot := incrementHash(functionSignaturesBaseSlot)
	packedData := state.GetState(p.contractAddr, packedSlot)

	// Extract isActive (lowest byte, rightmost bit)
	isActive := packedData[31]&0x01 == 1
	configAddr := common.BytesToAddress(packedData[12:32])

	// fmt.Println("- readConfigAtIndex -> rewardsLength:", rewardsLength)

	config := types.FeeMarketConfig{
		Events:             events,
		FunctionSignatures: nil,
		ConfigAddress:      configAddr,
		IsActive:           isActive,
	}

	// TODO: do we want to add a cap to rewardsLength?
	//       We can use the MAX_REWARD_ADDRESS from contract, though if we change this later, we will skip reading already set rewards in existing configs
	return config, nil
}

	// Read each reward
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
		rewardPercentage := uint256.NewInt(0).SetBytes(packedBytes[10:12])

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
