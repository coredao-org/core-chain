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

	// Calculate the storage slot for the array data
	// For dynamic arrays, the data starts at keccak256(slot)
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// For arrays of structs containing dynamic arrays, Solidity uses a more complex layout
	// Each Config is allocated a fixed amount of slots regardless of its actual Rewards array size

	// The size of each Config in slots, including space for fixed fields
	configSizeInSlots := uint64(4) // configRate, userConfigRate, isActive+configAddress (packed), rewards.length

	// Calculate the starting slot for this config
	indexOffset := new(big.Int).Mul(
		big.NewInt(int64(configSizeInSlots)),
		big.NewInt(int64(index)),
	)

	configSlot := common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(configsBaseSlot[:]),
		indexOffset,
	))

	// Each Config struct has the following fields in Solidity:
	// uint256 configRate - at slot + 0
	// uint256 userConfigRate - at slot + 1
	// bool isActive - at slot + 2 (packed with configAddress)
	// address configAddress - at slot + 2 (packed with isActive)
	// Reward[] rewards - at slot + 3 (dynamic array)

	// Read ConfigRate (at slot)
	configRateBytes := state.GetState(p.contractAddr, configSlot)
	configRate := new(uint256.Int).SetBytes(configRateBytes[:])
	// fmt.Println("- readConfigAtIndex -> configRate:", configRate)

	// Read UserConfigRate (at slot+1)
	userConfigRateSlot := incrementHash(configSlot)
	userConfigRateBytes := state.GetState(p.contractAddr, userConfigRateSlot)
	userConfigRate := new(uint256.Int).SetBytes(userConfigRateBytes[:])
	// fmt.Println("- readConfigAtIndex -> userConfigRate:", userConfigRate)

	// Read packed isActive and configAddress (at slot+2)
	// Solidity packs bool (1 byte) and address (20 bytes) into a single slot
	packedSlot := incrementHash(userConfigRateSlot)
	packedData := state.GetState(p.contractAddr, packedSlot)
	// fmt.Println("- readConfigAtIndex -> packedData:", packedData)

	// Extract isActive (lowest byte, rightmost bit)
	isActive := packedData[31]&0x01 == 1
	// fmt.Println("- readConfigAtIndex -> isActive:", isActive)

	// Extract configAddress (20 bytes, right-aligned)
	// In Solidity, the address is stored in the higher order bytes
	configAddr := common.BytesToAddress(packedData[11:31])
	// fmt.Println("- readConfigAtIndex -> configAddr:", configAddr)

	// Read the length of the Rewards array (at slot+3)
	rewardsSlot := incrementHash(packedSlot)
	rewardsLengthBytes := state.GetState(p.contractAddr, rewardsSlot)
	rewardsLength := new(uint256.Int).SetBytes(rewardsLengthBytes[:]).Uint64()
	// fmt.Println("- readConfigAtIndex -> rewardsLength:", rewardsLength)

	// Calculate base slot for rewards array data
	// For dynamic arrays, data is at keccak256(slot)
	rewardsBaseSlotBytes := crypto.Keccak256(rewardsSlot[:])
	rewardsBaseSlot := common.BytesToHash(rewardsBaseSlotBytes)

	// TODO: do we want to add a cap to rewardsLength?
	//       We can use the MAX_REWARD_ADDRESS from contract, though if we change this later, we will skip reading already set rewards in existing configs

	// Read each reward
	rewards := make([]types.FeeMarketReward, rewardsLength)
	for i := uint64(0); i < rewardsLength; i++ {
		rewardIndexBig := new(big.Int).SetUint64(i)
		rewardSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(rewardsBaseSlot[:]),
			rewardIndexBig,
		))

		// In Solidity, the Reward struct has:
		// address rewardAddress;
		// uint256 rewardPercentage;

		// Since address (20 bytes) and a uint256 (32 bytes) don't fit in one slot,
		// they will be stored in separate slots:
		// - rewardAddress at slot + 0
		// - rewardPercentage at slot + 1

		// Read reward address (at slot)
		rewardAddrBytes := state.GetState(p.contractAddr, rewardSlot)
		rewardAddr := common.BytesToAddress(rewardAddrBytes[:])
		// fmt.Println("- readConfigAtIndex -> reward[].address:", rewardAddr)

		// Read reward percentage (at slot+1)
		rewardPercentageSlot := incrementHash(rewardSlot)
		rewardPercentageBytes := state.GetState(p.contractAddr, rewardPercentageSlot)
		rewardPercentage := new(uint256.Int).SetBytes(rewardPercentageBytes[:])
		// fmt.Println("- readConfigAtIndex -> reward[].percentage:", rewardPercentage)

		rewards[i] = types.FeeMarketReward{
			RewardAddress:    rewardAddr,
			RewardPercentage: rewardPercentage,
		}
	}

	config := types.FeeMarketConfig{
		ConfigRate:     configRate,
		UserConfigRate: userConfigRate,
		IsActive:       isActive,
		ConfigAddress:  configAddr,
		Rewards:        rewards,
	}

	return config, nil
}

// incrementHash increments a hash value by 1
func incrementHash(h common.Hash) common.Hash {
	return common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(h[:]),
		big.NewInt(1),
	))
}
