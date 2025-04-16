package feemarket

import (
	"encoding/binary"
	"errors"
	"math/big"
	"math/rand"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type mockStateDB struct {
	storage map[common.Hash]common.Hash
}

func (m *mockStateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	return m.storage[key]
}

// TestIsValidConfig tests the isValidConfig function
func TestIsValidConfig(t *testing.T) {
	storage := map[common.Hash]common.Hash{}

	writeConstants(storage, types.FeeMarketConstants{
		MaxRewards: 2,
		MaxEvents:  2,
		MaxGas:     1000000,
	})

	stateDB := &mockStateDB{storage: storage}
	provider, err := NewFeeMarket()
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	testCases := []struct {
		name        string
		config      types.FeeMarketConfig
		expected    bool
		expectedErr error
	}{
		{
			name: "Valid config",
			config: types.FeeMarketConfig{
				IsActive:      true,
				ConfigAddress: common.HexToAddress("0x1234"),
				Events: []types.FeeMarketEvent{
					{
						EventSignature: common.Hash{1},
						Gas:            500000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 10000,
							},
						},
					},
				},
			},
			expected:    true,
			expectedErr: nil,
		},
		{
			name: "Invalid - Inactive config",
			config: types.FeeMarketConfig{
				IsActive:      false,
				ConfigAddress: common.HexToAddress("0x1234"),
			},
			expected:    false,
			expectedErr: errors.New("config is not active"),
		},
		{
			name: "Invalid - Too many events",
			config: types.FeeMarketConfig{
				IsActive:      true,
				ConfigAddress: common.HexToAddress("0x1234"),
				Events: []types.FeeMarketEvent{
					{
						EventSignature: common.Hash{1},
						Gas:            500000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 10000,
							},
						},
					},
					{
						EventSignature: common.Hash{2},
						Gas:            500000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 10000,
							},
						},
					},
					{
						EventSignature: common.Hash{3},
						Gas:            500000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 10000,
							},
						},
					},
				},
			},
			expected:    false,
			expectedErr: errors.New("invalid events length"),
		},
		{
			name: "Invalid - Gas too high",
			config: types.FeeMarketConfig{
				IsActive:      true,
				ConfigAddress: common.HexToAddress("0x1234"),
				Events: []types.FeeMarketEvent{
					{
						EventSignature: common.Hash{1},
						Gas:            2000000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 10000,
							},
						},
					},
				},
			},
			expected:    false,
			expectedErr: errors.New("invalid event gas"),
		},
		{
			name: "Invalid - Total percentage over denominator",
			config: types.FeeMarketConfig{
				IsActive:      true,
				ConfigAddress: common.HexToAddress("0x1234"),
				Events: []types.FeeMarketEvent{
					{
						EventSignature: common.Hash{1},
						Gas:            500000,
						Rewards: []types.FeeMarketReward{
							{
								RewardAddress:    common.HexToAddress("0x5678"),
								RewardPercentage: 6000,
							},
							{
								RewardAddress:    common.HexToAddress("0x9abc"),
								RewardPercentage: 5000,
							},
						},
					},
				},
			},
			expected:    false,
			expectedErr: errors.New("invalid total rewards percentage"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.config.IsValidConfig(
				provider.GetConstants(stateDB),
				DENOMINATOR,
			)
			if result != tc.expected {
				t.Errorf("Expected IsValidConfig to return %v, got %v", tc.expected, result)
			}
			if tc.expectedErr != nil && err.Error() != tc.expectedErr.Error() {
				t.Errorf("Expected error \"%v\", got \"%v\"", tc.expectedErr, err)
			}
		})
	}
}

// TestConstants tests the constants reading
func TestConstants(t *testing.T) {
	storage := map[common.Hash]common.Hash{}

	expectedConstants := types.FeeMarketConstants{
		MaxRewards: 10,
		MaxEvents:  20,
		MaxGas:     1000000,
	}
	writeConstants(storage, expectedConstants)

	stateDB := &mockStateDB{storage: storage}
	provider, err := NewFeeMarket()
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	actualConstants := provider.GetConstants(stateDB)
	if actualConstants != expectedConstants {
		t.Errorf("Expected constants %v, got %v", expectedConstants, actualConstants)
	}
}

func writeConstants(storage map[common.Hash]common.Hash, constants types.FeeMarketConstants) {
	// Pack the data into a 32 byte array
	packedData := make([]byte, 32)

	// Pack maxGas into last 4 bytes (uint32)
	binary.BigEndian.PutUint32(packedData[24:28], constants.MaxGas)
	packedData[29] = constants.MaxEvents
	packedData[30] = constants.MaxRewards

	storage[common.BigToHash(big.NewInt(CONSTANTS_STORAGE_SLOT))] = common.BytesToHash(packedData)
}

// writeRandomConfiguration writes a random config for the given address at the given index
func writeRandomConfiguration(storage map[common.Hash]common.Hash, addr common.Address, maxConstants types.FeeMarketConstants) types.FeeMarketConfig {
	constants := types.FeeMarketConstants{
		MaxEvents:  uint8(rand.Intn(int(maxConstants.MaxEvents))) + 1,
		MaxRewards: uint8(rand.Intn(int(maxConstants.MaxRewards))) + 1,
		MaxGas:     uint32(rand.Intn(int(maxConstants.MaxGas))) + 1,
	}
	return writeConfiguration(storage, addr, constants)
}

func writeConfiguration(storage map[common.Hash]common.Hash, addr common.Address, constants types.FeeMarketConstants) types.FeeMarketConfig {
	// Write the index in the address mapping
	configsMapSlot := common.BigToHash(big.NewInt(CONFIGS_MAP_STORAGE_SLOT))
	addressBytes := common.LeftPadBytes(addr.Bytes(), 32)
	data := append(addressBytes, configsMapSlot.Bytes()...)
	configSlot := common.BytesToHash(crypto.Keccak256(data))

	// Set up config data (isActive + address)
	packedData := make([]byte, 32)
	packedData[11] = 0x01 // isActive = true
	copy(packedData[12:32], addr.Bytes())
	storage[configSlot] = common.BytesToHash(packedData)

	// Set events length at slot+1
	eventsLengthSlot := incrementHash(configSlot)
	eventsLength := constants.MaxEvents
	storage[eventsLengthSlot] = common.BigToHash(big.NewInt(int64(eventsLength)))

	// Set up events data
	eventsBaseSlot := common.BytesToHash(crypto.Keccak256(eventsLengthSlot[:]))
	events := make([]types.FeeMarketEvent, eventsLength)

	// Create multiple events
	for eventIdx := uint8(0); eventIdx < constants.MaxEvents; eventIdx++ {
		eventSlot := common.BigToHash(new(big.Int).Add(
			new(big.Int).SetBytes(eventsBaseSlot.Bytes()),
			new(big.Int).Mul(big.NewInt(3), big.NewInt(int64(eventIdx))), // Each event takes 3 slots
		))

		// Generate random event data
		eventSig := common.Hash{byte(rand.Intn(255) + 1)}           // Random signature
		gas := uint64(rand.Intn(int(constants.MaxGas-1000))) + 1000 // Random gas between 1000 and maxGas

		// Each Event takes 3 slots (eventSignature, gas, rewards.length)
		eventSigSlot := eventSlot
		gasSlot := incrementHash(eventSigSlot)
		rewardsLengthSlot := incrementHash(gasSlot)

		// Set event data
		storage[eventSigSlot] = eventSig
		storage[gasSlot] = common.BigToHash(big.NewInt(int64(gas)))

		// Generate number of rewards
		rewards := make([]types.FeeMarketReward, constants.MaxRewards)
		storage[rewardsLengthSlot] = common.BigToHash(big.NewInt(int64(constants.MaxRewards)))

		// Set up reward data
		rewardsBaseSlot := common.BytesToHash(crypto.Keccak256(rewardsLengthSlot[:]))

		// Track total percentage to ensure it adds up to 10000
		remainingPercentage := uint64(10000)

		for i := uint64(0); i < uint64(constants.MaxRewards); i++ {
			rewardSlot := common.BigToHash(new(big.Int).Add(
				new(big.Int).SetBytes(rewardsBaseSlot.Bytes()),
				big.NewInt(int64(i)),
			))

			// Generate random reward data
			rewardAddr := common.BytesToAddress(crypto.Keccak256(append(addr.Bytes(), byte(i)))) // Unique per index

			var percentage uint64
			if i == uint64(constants.MaxRewards)-1 {
				percentage = remainingPercentage // Last reward gets remainder
			} else {
				maxPercent := remainingPercentage - (uint64(constants.MaxRewards) - i - 1) // Leave room for remaining rewards
				if maxPercent > 1 {
					percentage = uint64(rand.Intn(int(maxPercent-1))) + 1
				} else {
					percentage = 1
				}
				remainingPercentage -= percentage
			}

			// Set reward data (packed address + percentage)
			rewardData := make([]byte, 32)
			copy(rewardData[12:32], rewardAddr.Bytes())
			binary.BigEndian.PutUint16(rewardData[10:12], uint16(percentage))
			storage[rewardSlot] = common.BytesToHash(rewardData)

			rewards[i] = types.FeeMarketReward{
				RewardAddress:    rewardAddr,
				RewardPercentage: percentage,
			}
		}

		events[eventIdx] = types.FeeMarketEvent{
			EventSignature: eventSig,
			Gas:            gas,
			Rewards:        rewards,
		}
	}

	// Return the written config for verification
	return types.FeeMarketConfig{
		IsActive:      true,
		ConfigAddress: addr,
		Events:        events,
	}
}

// TestStorageLayoutParsing tests the storage layout parsing, it uses actual data as stored by the contract in the storage
func TestStorageLayoutParsing(t *testing.T) {
	// -- Setup storage with valid config and constants
	storage := map[common.Hash]common.Hash{
		// Config for address 0x96c4a1421b494e0cf1bb1e41911ec3251df94223
		// 1: configSlot [slot 0] (packed isActive and configAddress)
		common.HexToHash("0x12cb81277f1a78c5576703f63501cc1aedbad6b963375179202504604409aa43"): common.HexToHash("0x00000000000000000000000196c4a1421b494e0cf1bb1e41911ec3251df94223"),
		// 1: configSlot [slot 1] (eventsLengthSlot)
		common.HexToHash("0x12cb81277f1a78c5576703f63501cc1aedbad6b963375179202504604409aa44"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		common.HexToHash("0x405787fa12a823e0f2b7631cc41b3ba8828b3321ca811111fa75cd3aa3bb5ace"): common.HexToHash("0x00000000000000000000000096c4a1421b494e0cf1bb1e41911ec3251df94223"),
		// 1: configSlot -> 1: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x800db93068d4ee7373a17955cef4cfe1a5b2c98385893b58d38d4cc445bbf855"): common.HexToHash("0x0000000000000000000027108f10d3a6283672ecfaeea0377d460bded489ec44"),
		// 1: configSlot -> 1: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x950db65f3406dedf2d2f8f47af2ba44b624f998262d0229b91f7390977b7aefb"): common.HexToHash("0x51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a81"),
		// 1: configSlot -> 1: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x950db65f3406dedf2d2f8f47af2ba44b624f998262d0229b91f7390977b7aefc"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"),
		// 1: configSlot -> 1: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x950db65f3406dedf2d2f8f47af2ba44b624f998262d0229b91f7390977b7aefd"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),

		// Config for address 0x13261a11f2C6c6318240818de0Ddc3DB70a1B3bF
		// 2: configSlot [slot 0] (packed isActive and configAddress)
		common.HexToHash("0x0f2fd38231387d9f50b59f3719e56209cad04414f1b4ea7e4bb80e6e4e18043f"): common.HexToHash("0x00000000000000000000000113261a11f2c6c6318240818de0ddc3db70a1b3bf"),
		// 2: configSlot [slot 1] (eventsLengthSlot)
		common.HexToHash("0x0f2fd38231387d9f50b59f3719e56209cad04414f1b4ea7e4bb80e6e4e180440"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		// 2: configSlot -> 1: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x2b37d6b48c527b1c24c7b31e55dc893170bea392dddfd5eb522ac366d9024551"): common.HexToHash("0x0000000000000000000027108f10d3a6283672ecfaeea0377d460bded489ec44"),
		common.HexToHash("0x405787fa12a823e0f2b7631cc41b3ba8828b3321ca811111fa75cd3aa3bb5acf"): common.HexToHash("0x00000000000000000000000013261a11f2c6c6318240818de0ddc3db70a1b3bf"),
		// 2: configSlot -> 1: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x47efc7dc35c6613e58d6334258d2eb4097cf5686ff168d8e6e611c2ea5a793ef"): common.HexToHash("0x51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a81"),
		// 2: configSlot -> 1: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x47efc7dc35c6613e58d6334258d2eb4097cf5686ff168d8e6e611c2ea5a793f0"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"),
		// 2: configSlot -> 1: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x47efc7dc35c6613e58d6334258d2eb4097cf5686ff168d8e6e611c2ea5a793f1"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
	}

	constants := types.FeeMarketConstants{
		MaxRewards: 5,
		MaxEvents:  10,
		MaxGas:     1000000,
	}
	writeConstants(storage, constants)

	stateDB := &mockStateDB{storage: storage}
	fm, err := NewFeeMarket()
	if err != nil {
		t.Fatalf("Failed to create storage FeeMarket: %v", err)
	}

	contractAddr1 := common.HexToAddress("0x96c4a1421b494e0cf1bb1e41911ec3251df94223")
	if _, found := fm.GetActiveConfig(contractAddr1, stateDB); !found {
		t.Errorf("Config not found for address %s", contractAddr1.Hex())
	}

	contractAddr2 := common.HexToAddress("0x13261a11f2C6c6318240818de0Ddc3DB70a1B3bF")
	if _, found := fm.GetActiveConfig(contractAddr2, stateDB); !found {
		t.Errorf("Config not found for address %s", contractAddr2.Hex())
	}
	// Write generated config
	testAddr := common.HexToAddress("0x1234")
	genConfig1 := writeRandomConfiguration(storage, testAddr, constants)

	// Verify generated config can be read correctly
	config, found := fm.GetActiveConfig(testAddr, stateDB)
	if !found {
		t.Fatal("Generated config not found")
	}
	if !reflect.DeepEqual(config, genConfig1) {
		t.Errorf("Invalid generated config state: active=%v, addr=%s, expected addr=%s",
			config.IsActive, config.ConfigAddress.Hex(), genConfig1.ConfigAddress.Hex())
	}
}

// TestIncrementHash tests the incrementHash function
func TestIncrementHash(t *testing.T) {
	testCases := []struct {
		name     string
		input    common.Hash
		expected common.Hash
	}{
		{
			name:     "Zero hash",
			input:    common.Hash{},
			expected: common.BigToHash(big.NewInt(1)),
		},
		{
			name:     "Non-zero hash",
			input:    common.BigToHash(big.NewInt(42)),
			expected: common.BigToHash(big.NewInt(43)),
		},
		{
			name:     "Max uint256 minus 1",
			input:    common.HexToHash("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			expected: common.Hash{}, // Overflow to 0
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := incrementHash(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %x, got %x", tc.expected, result)
			}
		})
	}
}

// TestReadRewardsEdgeCases tests the readRewards function for edge cases
func TestReadRewardsEdgeCases(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	storage := make(map[common.Hash]common.Hash)
	stateDB := &mockStateDB{storage: storage}

	// Test empty rewards array
	rewardsLengthSlot := common.Hash{1}
	rewards := readRewards(configurationContractAddr, rewardsLengthSlot, 0, stateDB)
	if len(rewards) != 0 {
		t.Errorf("Expected empty rewards array, got length %d", len(rewards))
	}

	// Test rewards with max values
	rewardAddr := common.HexToAddress("0x1234")
	maxPercentage := uint64(65535) // max uint16
	packedData := make([]byte, 32)
	copy(packedData[12:32], rewardAddr.Bytes())
	binary.BigEndian.PutUint16(packedData[10:12], uint16(maxPercentage))

	rewardSlot := common.BytesToHash(crypto.Keccak256(rewardsLengthSlot[:]))
	stateDB.storage[rewardSlot] = common.BytesToHash(packedData)

	rewards = readRewards(configurationContractAddr, rewardsLengthSlot, 1, stateDB)
	if len(rewards) != 1 {
		t.Fatalf("Expected 1 reward, got %d", len(rewards))
	}
	if rewards[0].RewardAddress != rewardAddr {
		t.Errorf("Expected reward address %s, got %s", rewardAddr.Hex(), rewards[0].RewardAddress.Hex())
	}
	if rewards[0].RewardPercentage != maxPercentage {
		t.Errorf("Expected reward percentage %d, got %d", maxPercentage, rewards[0].RewardPercentage)
	}
}
