package feemarket

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/holiman/uint256"
)

const (
	// Storage slots for the fee market contract
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
}

// NewStorageProvider creates a new provider that reads directly from storage
func NewStorageProvider(contractAddr common.Address) (*StorageProvider, error) {
	return &StorageProvider{
		contractAddr: contractAddr,
	}, nil
}

// GetConstants reads the constants from the cache
func (p *StorageProvider) GetConstants(state StateReader) (constants types.FeeMarketConstants) {
	// TODO: what should happen if we can't load constants? or if some are 0?

	denominatorSlot := common.BigToHash(big.NewInt(DENOMINATOR_STORAGE_SLOT))
	denominatorBytes := state.GetState(p.contractAddr, denominatorSlot)
	constants.Denominator = new(uint256.Int).SetBytes(denominatorBytes[:]).Uint64()

	maxRewardsSlot := common.BigToHash(big.NewInt(MAX_REWARDS_STORAGE_SLOT))
	maxRewardsBytes := state.GetState(p.contractAddr, maxRewardsSlot)
	constants.MaxRewards = new(uint256.Int).SetBytes(maxRewardsBytes[:]).Uint64()

	maxGasSlot := common.BigToHash(big.NewInt(MAX_GAS_STORAGE_SLOT))
	maxGasBytes := state.GetState(p.contractAddr, maxGasSlot)
	constants.MaxGas = new(uint256.Int).SetBytes(maxGasBytes[:]).Uint64()

	maxEventsSlot := common.BigToHash(big.NewInt(MAX_EVENTS_STORAGE_SLOT))
	maxEventsBytes := state.GetState(p.contractAddr, maxEventsSlot)
	maxEvents := new(uint256.Int).SetBytes(maxEventsBytes[:]).Uint64()
	if maxEvents > math.MaxUint8 {
		maxEvents = math.MaxUint8
	}
	constants.MaxEvents = maxEvents

	maxFunctionSignaturesSlot := common.BigToHash(big.NewInt(MAX_FUNCTION_SIGNATURES_STORAGE_SLOT))
	maxFunctionSignaturesBytes := state.GetState(p.contractAddr, maxFunctionSignaturesSlot)
	maxFunctionSignatures := new(uint256.Int).SetBytes(maxFunctionSignaturesBytes[:]).Uint64()
	if maxFunctionSignatures > math.MaxUint8 {
		maxFunctionSignatures = math.MaxUint8
	}
	constants.MaxFunctionSignatures = maxFunctionSignatures

	return constants
}

// GetConfig returns configuration for a specific address
func (p *StorageProvider) GetConfig(address common.Address, state StateReader) (config types.FeeMarketConfig, found bool) {
	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	// Not found in cache, try to find it in storage
	config, found = p.findConfigForAddress(address, state)
	if !found {
		return types.FeeMarketConfig{}, false
	}

	// Cache storage happens in findConfigForAddress
	return config, true
}

// findConfigForAddress finds a config for an address by scanning all configs
func (p *StorageProvider) findConfigForAddress(address common.Address, state StateReader) (types.FeeMarketConfig, bool) {
	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	configsLength, err := p.readConfigsLength(state)
	if err != nil {
		log.Error("FeeMarket failed to read configs length", "err", err)
		return types.FeeMarketConfig{}, false
	}

	// Scan all configs to find one with matching address
	for i := uint64(0); i < configsLength; i++ {
		config, err := p.readConfigAtIndex(i, state)
		if err != nil {
			log.Debug("FeeMarket failed to read configuration", "index", i, "err", err)
			continue
		}

		// Validate config, invalid configs should not be cached either with wrong values.
		// TIP: In GetConfig, we will cache the invalid config as empty, but not as invalid
		constants := p.GetConstants(state)
		if valid, err := config.IsValidConfig(constants); !valid || err != nil {
			log.Debug("FeeMarket invalid config found in storage", "config", config, "err", err)
			return types.FeeMarketConfig{}, false
		}

		if config.ConfigAddress == address {
			return config, true
		}
	}

	return types.FeeMarketConfig{}, false
}

// readConfigsLength reads the length of the configs array from storage
func (p *StorageProvider) readConfigsLength(state StateReader) (uint64, error) {
	// In Solidity, the length of a public array is stored at the array's slot
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	lengthBytes := state.GetState(p.contractAddr, configsSlot)
	return new(uint256.Int).SetBytes(lengthBytes[:]).Uint64(), nil
}

// readConfigAtIndex reads a config from storage at a specific index
func (p *StorageProvider) readConfigAtIndex(index uint64, state StateReader) (config types.FeeMarketConfig, err error) {
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

	// Read events array length (right-aligned)
	eventsLengthSlot := incrementHash(packedSlot)
	eventsLengthData := state.GetState(p.contractAddr, eventsLengthSlot)
	eventsLength := new(uint256.Int).SetBytes(eventsLengthData.Bytes()).Uint64()

	constants := p.GetConstants(state)
	if eventsLength > constants.MaxEvents {
		log.Error("FeeMarket events length is greater than max events", "index", index, "eventsLength", eventsLength, "maxEvents", constants.MaxEvents)
		return types.FeeMarketConfig{}, fmt.Errorf("events length is greater than max events")
	}

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
		if rewardsLength > constants.MaxRewards {
			log.Error("FeeMarket rewards length is greater than max rewards", "index", index, "rewardsLength", rewardsLength, "maxRewards", constants.MaxRewards)
			return types.FeeMarketConfig{}, fmt.Errorf("rewards length is greater than max rewards")
		}

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
func readRewards(contractAddr common.Address, rewardsLengthSlot common.Hash, rewardsLength uint64, state StateReader) []types.FeeMarketReward {
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
