// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

const (
	// Storage slots for the fee market contract
	CONSTANTS_STORAGE_SLOT   = 0
	CONFIGS_MAP_STORAGE_SLOT = 1

	// The denominator for the fee market
	DENOMINATOR = uint16(10000)
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

// GetConstants reads the contracts constants
func (fm *FeeMarket) GetConstants(state StateReader) types.FeeMarketConstants {
	constantsSlot := common.BigToHash(big.NewInt(CONSTANTS_STORAGE_SLOT))
	constantsBytes := state.GetState(fm.contractAddress, constantsSlot)

	maxGas := binary.BigEndian.Uint32(constantsBytes[24:28])
	maxEvents := constantsBytes[29]
	maxRewards := constantsBytes[30]

	return types.FeeMarketConstants{
		MaxRewards: maxRewards,
		MaxEvents:  maxEvents,
		MaxGas:     maxGas,
	}
}

// GetActiveConfig returns configuration for a specific address
func (fm *FeeMarket) GetActiveConfig(address common.Address, state StateReader) (config types.FeeMarketConfig, gas uint64, found bool) {
	if state == nil {
		return types.FeeMarketConfig{}, 0, false
	}

	config, gas, err := fm.readConfigForAddress(address, state)
	if err != nil {
		return types.FeeMarketConfig{}, gas, false
	}

	// Validate the config
	if valid, err := config.IsValidConfig(fm.GetConstants(state), DENOMINATOR); !valid || err != nil {
		log.Debug("FeeMarket invalid config found in mapping", "address", address, "config", config, "err", err)
		return types.FeeMarketConfig{}, gas, false
	}

	return config, gas, true
}

// readConfigForAddress reads a config from storage for a specific address
func (fm *FeeMarket) readConfigForAddress(address common.Address, state StateReader) (config types.FeeMarketConfig, gas uint64, err error) {
	readStateFn := func(slot common.Hash) common.Hash {
		// Add Sload gas for each GetState read
		gas += params.FeeMarketSloadGas
		return state.GetState(fm.contractAddress, slot)
	}

	configsMapSlot := common.BigToHash(big.NewInt(CONFIGS_MAP_STORAGE_SLOT))

	// Prepare the input for keccak256: address (padded to 32 bytes) + slot
	// In Solidity, addresses are left-padded in storage
	addressBytes := common.LeftPadBytes(address.Bytes(), 32)
	data := append(addressBytes, configsMapSlot.Bytes()...)
	configSlot := common.BytesToHash(crypto.Keccak256(data))

	// Read packed isActive and configAddress (at slot+2)
	// Solidity packs bool (1 byte) and address (20 bytes) into a single slot
	packedSlot := configSlot
	packedData := readStateFn(packedSlot)

	// Extract isActive (lowest byte, rightmost bit)
	isActive := packedData[11]&0x01 == 1
	configAddr := common.BytesToAddress(packedData[12:32])

	if configAddr == (common.Address{}) {
		return types.FeeMarketConfig{}, gas, fmt.Errorf("no configuration found for this slot index")
	}

	// Get the constants from the contract
	constants := fm.GetConstants(state)

	// Read events array length (right-aligned)
	eventsLengthSlot := incrementHash(packedSlot)
	eventsLengthData := readStateFn(eventsLengthSlot)
	eventsLength := new(uint256.Int).SetBytes(eventsLengthData.Bytes()).Uint64()

	if eventsLength > math.MaxUint8 || uint8(eventsLength) > constants.MaxEvents {
		log.Error("FeeMarket events length is greater than max events", "address", address, "eventsLength", eventsLength, "maxEvents", constants.MaxEvents)
		return types.FeeMarketConfig{}, gas, fmt.Errorf("events length is greater than max events")
	}

	// Read events
	eventsBaseSlot := common.BytesToHash(crypto.Keccak256(eventsLengthSlot[:]))
	events := make([]types.FeeMarketEvent, eventsLength)
	for i := uint8(0); i < uint8(eventsLength); i++ {
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
			readStateFn(rewardsLengthSlot).Bytes(),
		).Uint64()

		if rewardsLength > math.MaxUint8 || uint8(rewardsLength) > constants.MaxRewards {
			log.Error("FeeMarket rewards length is greater than max rewards", "address", address, "rewardsLength", rewardsLength, "maxRewards", constants.MaxRewards)
			return types.FeeMarketConfig{}, gas, fmt.Errorf("rewards length is greater than max rewards")
		}

		rewards, rewardsGas := readRewards(fm.contractAddress, rewardsLengthSlot, uint8(rewardsLength), state)
		gas += rewardsGas

		events[i] = types.FeeMarketEvent{
			EventSignature: readStateFn(eventSigSlot),
			Gas:            binary.BigEndian.Uint32(readStateFn(gasSlot).Bytes()[28:32]),
			Rewards:        rewards,
		}
	}

	config = types.FeeMarketConfig{
		ConfigAddress: configAddr,
		IsActive:      isActive,
		Events:        events,
	}

	return config, gas, nil
}

// Helper function to read rewards array
func readRewards(contractAddr common.Address, rewardsLengthSlot common.Hash, rewardsLength uint8, state StateReader) (rewards []types.FeeMarketReward, gas uint64) {
	rewardsBaseSlot := common.BytesToHash(crypto.Keccak256(rewardsLengthSlot[:]))
	rewards = make([]types.FeeMarketReward, rewardsLength)

	for i := uint8(0); i < rewardsLength; i++ {
		// Each Reward takes 1 slot with packed fields (rewardAddr, rewardPercentage)
		rewardSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(rewardsBaseSlot[:]),
			big.NewInt(int64(i)),
		))

		// First 20 bytes for the address
		// Last 2 bytes for the percentage (uint16)
		// Solidity packs data right-aligned, so we need to read from the end
		packedBytes := state.GetState(contractAddr, rewardSlot)

		// Add Sload gas for each GetState read
		gas += params.FeeMarketSloadGas

		rewardAddress := common.BytesToAddress(packedBytes[12:32])
		rewardPercentage := binary.BigEndian.Uint16(packedBytes[10:12])

		rewards[i] = types.FeeMarketReward{
			RewardAddress:    rewardAddress,
			RewardPercentage: rewardPercentage,
		}
	}
	return rewards, gas
}

// incrementHash increments a hash value by 1
func incrementHash(h common.Hash) common.Hash {
	return common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(h[:]),
		big.NewInt(1),
	))
}
