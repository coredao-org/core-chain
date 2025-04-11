// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
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
	CONFIGS_MAP_STORAGE_SLOT             = 7
)

// FeeMarket represents the fee market integration which is used to get the fee market config for an address.
// It directly reads from storage to avoid the overhead of calling the contract.
type FeeMarket struct {
	// contractAddress is the address of the FeeMarketContract
	contractAddress common.Address
}

// NewFeeMarket creates a new fee market integration using storage access
func NewFeeMarket() (*FeeMarket, error) {
	feeMarketContractAddress := common.HexToAddress(systemcontracts.FeeMarketContract)
	return &FeeMarket{
		contractAddress: feeMarketContractAddress,
	}, nil
}

// GetDenominator reads the denominator from contracts constants
func (fm *FeeMarket) GetDenominator(state StateReader) uint64 {
	denominatorSlot := common.BigToHash(big.NewInt(DENOMINATOR_STORAGE_SLOT))
	denominatorBytes := state.GetState(fm.contractAddress, denominatorSlot)
	return new(uint256.Int).SetBytes(denominatorBytes[:]).Uint64()
}

// GetMaxRewards reads the max rewards from contracts constants
func (fm *FeeMarket) GetMaxRewards(state StateReader) uint64 {
	maxRewardsSlot := common.BigToHash(big.NewInt(MAX_REWARDS_STORAGE_SLOT))
	maxRewardsBytes := state.GetState(fm.contractAddress, maxRewardsSlot)
	return new(uint256.Int).SetBytes(maxRewardsBytes[:]).Uint64()
}

// GetMaxGas reads the max gas from contracts constants
func (fm *FeeMarket) GetMaxGas(state StateReader) uint64 {
	maxGasSlot := common.BigToHash(big.NewInt(MAX_GAS_STORAGE_SLOT))
	maxGasBytes := state.GetState(fm.contractAddress, maxGasSlot)
	return new(uint256.Int).SetBytes(maxGasBytes[:]).Uint64()
}

// GetMaxEvents reads the max events from contracts constants
func (fm *FeeMarket) GetMaxEvents(state StateReader) uint64 {
	maxEventsSlot := common.BigToHash(big.NewInt(MAX_EVENTS_STORAGE_SLOT))
	maxEventsBytes := state.GetState(fm.contractAddress, maxEventsSlot)
	maxEvents := new(uint256.Int).SetBytes(maxEventsBytes[:]).Uint64()
	if maxEvents > math.MaxUint8 {
		maxEvents = math.MaxUint8
	}
	return maxEvents
}

// GetMaxFunctionSignatures reads the max function signatures from contracts constants
func (fm *FeeMarket) GetMaxFunctionSignatures(state StateReader) uint64 {
	maxFunctionSignaturesSlot := common.BigToHash(big.NewInt(MAX_FUNCTION_SIGNATURES_STORAGE_SLOT))
	maxFunctionSignaturesBytes := state.GetState(fm.contractAddress, maxFunctionSignaturesSlot)
	maxFunctionSignatures := new(uint256.Int).SetBytes(maxFunctionSignaturesBytes[:]).Uint64()
	if maxFunctionSignatures > math.MaxUint8 {
		maxFunctionSignatures = math.MaxUint8
	}
	return maxFunctionSignatures
}

// GetConfig returns configuration for a specific address
func (fm *FeeMarket) GetConfig(address common.Address, state StateReader) (config types.FeeMarketConfig, found bool) {
	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	config, found = fm.findConfigForAddressFromMap(address, state)
	if !found {
		return types.FeeMarketConfig{}, false
	}

	// Validate the config
	denominator := fm.GetDenominator(state)
	maxGas := fm.GetMaxGas(state)
	maxEvents := fm.GetMaxEvents(state)
	maxRewards := fm.GetMaxRewards(state)
	if valid, err := config.IsValidConfig(denominator, maxGas, maxEvents, maxRewards); !valid || err != nil {
		log.Debug("FeeMarket invalid config found in mapping", "address", address, "config", config, "err", err)
		return types.FeeMarketConfig{}, false
	}

	return config, true
}

// findConfigForAddressFromMap returns a configuration for a specific address using the mapping
func (fm *FeeMarket) findConfigForAddressFromMap(address common.Address, state StateReader) (types.FeeMarketConfig, bool) {
	// Calculate the storage slot for the mapping
	configsMapSlot := common.BigToHash(big.NewInt(CONFIGS_MAP_STORAGE_SLOT))

	// Prepare the input for keccak256: address (padded to 32 bytes) + slot
	// In Solidity, addresses are left-padded in storage
	addressBytes := common.LeftPadBytes(address.Bytes(), 32)
	data := append(addressBytes, configsMapSlot.Bytes()...)

	// Calculate the storage slot using keccak256
	mappingSlot := common.BytesToHash(crypto.Keccak256(data))

	// Get the index from the mapping
	indexBytes := state.GetState(fm.contractAddress, mappingSlot)
	index := new(uint256.Int).SetBytes(indexBytes[:]).Uint64()

	// Read the config at the found index
	config, err := fm.readConfigAtIndex(index, state)
	if err != nil {
		log.Debug("FeeMarket failed to read configuration", "index", index, "err", err)
		return types.FeeMarketConfig{}, false
	}

	// Double-check that the config address matches the requested address
	if config.ConfigAddress != address {
		log.Debug("FeeMarket mapping returned wrong config", "requested", address, "found", config.ConfigAddress)
		return types.FeeMarketConfig{}, false
	}

	return config, true
}

// findConfigForAddressFromArray finds a config for an address by scanning all configs
func (fm *FeeMarket) findConfigForAddressFromArray(address common.Address, state StateReader) (types.FeeMarketConfig, bool) {
	if state == nil {
		return types.FeeMarketConfig{}, false
	}

	configsLength, err := fm.readConfigsLength(state)
	if err != nil {
		log.Debug("FeeMarket failed to read configs length", "err", err)
		return types.FeeMarketConfig{}, false
	}

	// Scan all configs to find one with matching address
	for i := uint64(0); i < configsLength; i++ {
		config, err := fm.readConfigAtIndex(i, state)
		if err != nil {
			log.Debug("FeeMarket failed to read configuration", "index", i, "err", err)
			continue
		}

		if config.ConfigAddress == address {
			return config, true
		}
	}

	return types.FeeMarketConfig{}, false
}

// readConfigsLength reads the length of the configs array from storage
func (fm *FeeMarket) readConfigsLength(state StateReader) (uint64, error) {
	// In Solidity, the length of a public array is stored at the array's slot
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	lengthBytes := state.GetState(fm.contractAddress, configsSlot)
	return new(uint256.Int).SetBytes(lengthBytes[:]).Uint64(), nil
}

// readConfigAtIndex reads a config from storage at a specific index
func (fm *FeeMarket) readConfigAtIndex(index uint64, state StateReader) (config types.FeeMarketConfig, err error) {
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
	packedData := state.GetState(fm.contractAddress, packedSlot)

	// Extract isActive (lowest byte, rightmost bit)
	isActive := packedData[11]&0x01 == 1
	configAddr := common.BytesToAddress(packedData[12:32])

	// Read events array length (right-aligned)
	eventsLengthSlot := incrementHash(packedSlot)
	eventsLengthData := state.GetState(fm.contractAddress, eventsLengthSlot)
	eventsLength := new(uint256.Int).SetBytes(eventsLengthData.Bytes()).Uint64()

	maxEvents := fm.GetMaxEvents(state)
	if eventsLength > maxEvents {
		log.Error("FeeMarket events length is greater than max events", "index", index, "eventsLength", eventsLength, "maxEvents", maxEvents)
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
			state.GetState(fm.contractAddress, rewardsLengthSlot).Bytes(),
		).Uint64()

		maxRewards := fm.GetMaxRewards(state)
		if rewardsLength > maxRewards {
			log.Error("FeeMarket rewards length is greater than max rewards", "index", index, "rewardsLength", rewardsLength, "maxRewards", maxRewards)
			return types.FeeMarketConfig{}, fmt.Errorf("rewards length is greater than max rewards")
		}

		rewards := readRewards(fm.contractAddress, rewardsLengthSlot, rewardsLength, state)

		events[i] = types.FeeMarketEvent{
			EventSignature: state.GetState(fm.contractAddress, eventSigSlot),
			Gas:            new(uint256.Int).SetBytes(state.GetState(fm.contractAddress, gasSlot).Bytes()).Uint64(),
			Rewards:        rewards,
		}
	}

	// Function signatures will be handled in a later release.
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
