package feemarket

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"
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

func TestStorageLayoutParsing(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	contractAddr1 := common.HexToAddress("0xe452e78f45ed9542be008308ebdb7d13e786884b")
	contractAddr2 := common.HexToAddress("0xBD673746fD7Da230489AEee913922467b543ab54")
	storage := map[common.Hash]common.Hash{
		// constants
		common.HexToHash("0x0000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"),
		common.HexToHash("0x0000000000000000000000000000000000000002"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		common.HexToHash("0x0000000000000000000000000000000000000003"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000003"),
		common.HexToHash("0x0000000000000000000000000000000000000004"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000004"),
		common.HexToHash("0x0000000000000000000000000000000000000005"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000f4240"),

		// configs
		common.HexToHash("0x0000000000000000000000000000000000000006"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),

		// 1: configSlot -> 2: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x36049a515b56f00dd3fda789341a520e4cd9943bb902ddb34aaf2985b96b84f7"): common.HexToHash("0x0000000000000000000023288f10d3a6283672ecfaeea0377d460bded489ec44"),
		// 1: configSlot -> 2: eventSlot -> 2: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x36049a515b56f00dd3fda789341a520e4cd9943bb902ddb34aaf2985b96b84f8"): common.HexToHash("0x0000000000000000000003e80000000000000000000000000000000000000789"),
		// 1: configSlot -> 1: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce1"): common.HexToHash("0x51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a81"),
		// 1: configSlot -> 1: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce2"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000186a0"),
		// 1: configSlot -> 1: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce3"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// 1: configSlot -> 2: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce4"): common.HexToHash("0x0335b51418df6ad87c7638414b2dd16910635533ebf9090fab3f0fdd07a51508"),
		// 1: configSlot -> 2: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce5"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000030d40"),
		// 1: configSlot -> 2: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x768c3a22b1e4688c94525eb9bc2cf1ce7601fc9e871dc6e10fc44f0f06340ce6"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// 1: configSlot -> 1: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x826e18df85dad61eff35b9d11c2a9631949d2ab75d01408a39e1f3400d7f198c"): common.HexToHash("0x0000000000000000000023288f10d3a6283672ecfaeea0377d460bded489ec44"),
		// 1: configSlot -> 1: eventSlot -> 2: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x826e18df85dad61eff35b9d11c2a9631949d2ab75d01408a39e1f3400d7f198d"): common.HexToHash("0x0000000000000000000003e80000000000000000000000000000000000000789"),
		// 1: configSlot [slot 0] (packed isActive and configAddress)
		common.HexToHash("0xf652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d3f"): common.HexToHash("0x000000000000000000000001e452e78f45ed9542be008308ebdb7d13e786884b"),
		// 1: configSlot [slot 1] (eventsLengthSlot)
		common.HexToHash("0xf652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d40"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),

		// 2nd discount
		// 2: configSlot -> 2: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x0b6863eba6c023daa40fb79ecc1f6091ccda27004508009816639ff2cad368bd"): common.HexToHash("0x0000000000000000000023280000000000000000000000000000000000000123"),
		// 2: configSlot -> 2: eventSlot -> 2: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0x0b6863eba6c023daa40fb79ecc1f6091ccda27004508009816639ff2cad368be"): common.HexToHash("0x0000000000000000000003e80000000000000000000000000000000000000456"),
		// 2: configSlot -> 1: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0a5"): common.HexToHash("0x51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a81"),
		// 2: configSlot -> 1: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0a6"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000186a0"),
		// 2: configSlot -> 1: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0a7"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// 2: configSlot -> 2: eventSlot [slot 0] (eventSigSlot)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0a8"): common.HexToHash("0x0335b51418df6ad87c7638414b2dd16910635533ebf9090fab3f0fdd07a51508"),
		// 2: configSlot -> 2: eventSlot [slot 1] (gasSlot)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0a9"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000030d40"),
		// 2: configSlot -> 2: eventSlot [slot 2] (rewardsLength)
		common.HexToHash("0x35817d789b7a6dbe8b95b0f21e189fb26d3d329de699cac7a267a9568298e0aa"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// 2: configSlot [slot 0] (packed isActive and configAddress)
		common.HexToHash("0xf652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d42"): common.HexToHash("0x000000000000000000000001bd673746fd7da230489aeee913922467b543ab54"),
		// 2: configSlot [slot 1] (eventsLengthSlot)
		common.HexToHash("0xf652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d43"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// 2: configSlot -> 1: eventSlot -> 1: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0xf877648a5fd5598677ea6f98c3662fd95b67a58762d49d5f3c34688bc14cc0f5"): common.HexToHash("0x0000000000000000000023280000000000000000000000000000000000000123"),
		// 2: configSlot -> 1: eventSlot -> 2: rewardSlot [slot 0] (packed rewardAddr + rewardPercentage)
		common.HexToHash("0xf877648a5fd5598677ea6f98c3662fd95b67a58762d49d5f3c34688bc14cc0f6"): common.HexToHash("0x0000000000000000000003e80000000000000000000000000000000000000456"),
	}

	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create storage provider: %v", err)
	}

	stateDB := &mockStateDB{storage: storage}
	if _, found := provider.GetConfig(contractAddr1, stateDB, false); !found {
		t.Errorf("Config not found")
	}

	if _, found := provider.GetConfig(contractAddr2, stateDB, false); !found {
		t.Errorf("Config not found")
	}

	// TODO: validate the config values as well
}

// TestIsValidConfig tests the isValidConfig function
func TestIsValidConfig(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	storage := map[common.Hash]common.Hash{
		common.HexToHash("0x0000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"), // denominator = 10000
		common.HexToHash("0x0000000000000000000000000000000000000002"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"), // maxRewards = 2
		common.HexToHash("0x0000000000000000000000000000000000000003"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"), // maxEvents = 2
		common.HexToHash("0x0000000000000000000000000000000000000005"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000f4240"), // maxGas = 1000000
	}

	stateDB := &mockStateDB{storage: storage}
	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	testCases := []struct {
		name     string
		config   types.FeeMarketConfig
		expected bool
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
			expected: true,
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
			expected: false,
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
			expected: false,
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
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.config.IsValidConfig(
				provider.GetDenominator(stateDB, false),
				provider.GetMaxGas(stateDB, false),
				provider.GetMaxEvents(stateDB, false),
				provider.GetMaxRewards(stateDB, false),
			)
			if result != tc.expected {
				t.Errorf("Expected IsValidConfig to return %v, got %v", tc.expected, result)
			}
		})
	}
}

// TestHandleCacheInvalidation tests the cache invalidation for a specific config as well as all configs
func TestHandleCacheInvalidation(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	testAddr := common.HexToAddress("0x1234")

	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Add test configs
	testConfig := types.FeeMarketConfig{
		IsActive:      true,
		ConfigAddress: testAddr,
	}
	provider.configCache[testAddr] = testConfig

	// Test invalidating specific config
	provider.InvalidateConfig(testAddr)
	if _, exists := provider.configCache[testAddr]; exists {
		t.Error("Config should have been removed from cache")
	}

	// Test invalidating all configs
	provider.configCache[testAddr] = testConfig
	provider.CleanConfigsCache()
	if len(provider.configCache) != 0 {
		t.Error("Cache should be empty after cleaning")
	}
}

// TestConstantsCache tests the constants cache
func TestConstantsCache(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")

	type testCase struct {
		name        string
		initialVal  uint64
		newVal      uint64
		storageSlot common.Hash
		getter      func(*StorageProvider, FeeMarketStateReader, bool) uint64
	}

	testCases := []testCase{
		{
			name:        "Denominator",
			initialVal:  10000,
			newVal:      5000,
			storageSlot: common.BigToHash(big.NewInt(DENOMINATOR_STORAGE_SLOT)),
			getter:      (*StorageProvider).GetDenominator,
		},
		{
			name:        "MaxRewards",
			initialVal:  5,
			newVal:      10,
			storageSlot: common.BigToHash(big.NewInt(MAX_REWARDS_STORAGE_SLOT)),
			getter:      (*StorageProvider).GetMaxRewards,
		},
		{
			name:        "MaxEvents",
			initialVal:  6,
			newVal:      12,
			storageSlot: common.BigToHash(big.NewInt(MAX_EVENTS_STORAGE_SLOT)),
			getter:      (*StorageProvider).GetMaxEvents,
		},
		{
			name:        "MaxGas",
			initialVal:  1000000,
			newVal:      2000000,
			storageSlot: common.BigToHash(big.NewInt(MAX_GAS_STORAGE_SLOT)),
			getter:      (*StorageProvider).GetMaxGas,
		},
		{
			name:        "MaxFunctionSignatures",
			initialVal:  7,
			newVal:      14,
			storageSlot: common.BigToHash(big.NewInt(MAX_FUNCTION_SIGNATURES_STORAGE_SLOT)),
			getter:      (*StorageProvider).GetMaxFunctionSignatures,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			storage := map[common.Hash]common.Hash{
				tc.storageSlot: common.BigToHash(big.NewInt(int64(tc.initialVal))),
			}
			stateDB := &mockStateDB{storage: storage}
			provider, err := NewStorageProvider(configurationContractAddr)
			if err != nil {
				t.Fatalf("Failed to create provider: %v", err)
			}

			// Test initial read and cache
			val := tc.getter(provider, stateDB, true)
			if val != tc.initialVal {
				t.Errorf("Expected initial %s to be %d, got %d", tc.name, tc.initialVal, val)
			}

			// Modify storage but keep cache
			stateDB.storage[tc.storageSlot] = common.BigToHash(big.NewInt(int64(tc.newVal)))

			// Should still get new value, by ingoring the cache, not clearing it
			val = tc.getter(provider, stateDB, false)
			if val != tc.newVal {
				t.Errorf("Expected new %s to be %d, got %d", tc.name, tc.newVal, val)
			}

			// Should still get previously cached value
			val = tc.getter(provider, stateDB, true)
			if val != tc.initialVal {
				t.Errorf("Expected cached %s to be %d, got %d", tc.name, tc.initialVal, val)
			}

			// Invalidate cache
			provider.InvalidateConstants()

			// Should get new value
			val = tc.getter(provider, stateDB, true)
			if val != tc.newVal {
				t.Errorf("Expected new %s to be %d, got %d", tc.name, tc.newVal, val)
			}
		})
	}
}

// TestConcurrentAccess tests the concurrent access to the storage
func TestConcurrentAccess(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	testAddr := common.HexToAddress("0x1234")

	DENOMINATOR_VALUE := uint64(10000)
	MAX_REWARDS_VALUE := uint64(5)
	MAX_EVENTS_VALUE := uint64(10)
	MAX_GAS_VALUE := uint64(1000000)
	MAX_FUNCTION_SIGNATURES_VALUE := uint64(20)

	// -- Setup storage with valid config and constants
	storage := map[common.Hash]common.Hash{
		// Constants
		common.BigToHash(big.NewInt(DENOMINATOR_STORAGE_SLOT)):             common.BigToHash(big.NewInt(int64(DENOMINATOR_VALUE))),
		common.BigToHash(big.NewInt(MAX_REWARDS_STORAGE_SLOT)):             common.BigToHash(big.NewInt(int64(MAX_REWARDS_VALUE))),
		common.BigToHash(big.NewInt(MAX_EVENTS_STORAGE_SLOT)):              common.BigToHash(big.NewInt(int64(MAX_EVENTS_VALUE))),
		common.BigToHash(big.NewInt(MAX_GAS_STORAGE_SLOT)):                 common.BigToHash(big.NewInt(int64(MAX_GAS_VALUE))),
		common.BigToHash(big.NewInt(MAX_FUNCTION_SIGNATURES_STORAGE_SLOT)): common.BigToHash(big.NewInt(int64(MAX_FUNCTION_SIGNATURES_VALUE))),
	}

	// Setup a valid config in storage
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	storage[configsSlot] = common.BigToHash(big.NewInt(1)) // One config

	// Calculate base slot for configs array
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// Each Config takes 3 slots (packed fields, events.length, functionSignatures.length)
	configSizeInSlots := uint64(3)

	// Calculate this config's starting slot (index 0)
	indexOffset := new(big.Int).Mul(
		big.NewInt(int64(configSizeInSlots)),
		big.NewInt(0), // index 0
	)
	configSlot := common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(configsBaseSlot[:]),
		indexOffset,
	))

	// Set up config data (isActive + address)
	packedData := make([]byte, 32)
	packedData[11] = 0x01 // isActive = true
	copy(packedData[12:32], testAddr.Bytes())
	storage[configSlot] = common.BytesToHash(packedData)

	// Set events length at slot+1
	eventsLengthSlot := incrementHash(configSlot)
	storage[eventsLengthSlot] = common.BigToHash(big.NewInt(1))

	// Set up event data
	eventsBaseSlot := common.BytesToHash(crypto.Keccak256(eventsLengthSlot[:]))
	eventSlot := common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(eventsBaseSlot.Bytes()),
		big.NewInt(0), // index 0
	))

	// Each Event takes 3 slots (eventSignature, gas, rewards.length)
	eventSigSlot := eventSlot
	gasSlot := incrementHash(eventSigSlot)
	rewardsLengthSlot := incrementHash(gasSlot)

	// Set event data
	storage[eventSigSlot] = common.Hash{1}                       // Some event signature
	storage[gasSlot] = common.BigToHash(big.NewInt(100000))      // Gas limit
	storage[rewardsLengthSlot] = common.BigToHash(big.NewInt(1)) // One reward

	// Set up reward data
	rewardsBaseSlot := common.BytesToHash(crypto.Keccak256(rewardsLengthSlot[:]))
	rewardSlot := common.BigToHash(new(big.Int).Add(
		new(big.Int).SetBytes(rewardsBaseSlot.Bytes()),
		big.NewInt(0), // index 0
	))

	// Set reward data (packed address + percentage)
	rewardData := make([]byte, 32)
	rewardAddr := common.HexToAddress("0x5678")
	copy(rewardData[12:32], rewardAddr.Bytes())
	rewardData[10] = 0x27 // 10000 in uint16
	rewardData[11] = 0x10
	storage[rewardSlot] = common.BytesToHash(rewardData)

	t.Logf("Storage setup verification:")
	t.Logf("Contract address: %s", configurationContractAddr.Hex())
	t.Logf("Test address: %s", testAddr.Hex())
	t.Logf("Config slot: %s", configSlot.Hex())
	t.Logf("Events length slot: %s", eventsLengthSlot.Hex())

	// -- Start testing
	stateDB := &mockStateDB{storage: storage}
	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Verify constants are read correctly
	denominator := provider.GetDenominator(stateDB, true)
	if denominator != DENOMINATOR_VALUE {
		t.Errorf("Invalid denominator: got %d, want %d", denominator, DENOMINATOR_VALUE)
	}
	maxRewards := provider.GetMaxRewards(stateDB, true)
	if maxRewards != MAX_REWARDS_VALUE {
		t.Errorf("Invalid maxRewards: got %d, want %d", maxRewards, MAX_REWARDS_VALUE)
	}
	maxEvents := provider.GetMaxEvents(stateDB, true)
	if maxEvents != MAX_EVENTS_VALUE {
		t.Errorf("Invalid maxEvents: got %d, want %d", maxEvents, MAX_EVENTS_VALUE)
	}
	maxGas := provider.GetMaxGas(stateDB, true)
	if maxGas != MAX_GAS_VALUE {
		t.Errorf("Invalid maxGas: got %d, want %d", maxGas, MAX_GAS_VALUE)
	}
	maxFunctionSignatures := provider.GetMaxFunctionSignatures(stateDB, true)
	if maxFunctionSignatures != MAX_FUNCTION_SIGNATURES_VALUE {
		t.Errorf("Invalid maxFunctionSignatures: got %d, want %d", maxFunctionSignatures, MAX_FUNCTION_SIGNATURES_VALUE)
	}

	// Try to read the config directly first
	config, found := provider.GetConfig(testAddr, stateDB, true)
	if !found {
		t.Log("Direct config read failed")
	}

	// Setup concurrent access test
	var wg sync.WaitGroup
	errorChan := make(chan error, 200) // Buffer for 100 readers + 100 writers
	concurrentAccesses := 100

	// Start readers
	for i := 0; i < concurrentAccesses; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			config, found := provider.GetConfig(testAddr, stateDB, true)
			if !found {
				errorChan <- fmt.Errorf("config not found")
				return
			}
			if config.ConfigAddress != testAddr {
				errorChan <- fmt.Errorf("wrong address, got %s, want %s", config.ConfigAddress.Hex(), testAddr.Hex())
				return
			}
		}()
	}

	// Start writers
	for i := 0; i < concurrentAccesses; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider.InvalidateConfig(testAddr)
			config, found := provider.GetConfig(testAddr, stateDB, true)
			if !found {
				errorChan <- fmt.Errorf("config not found")
				return
			}
			if config.ConfigAddress != testAddr {
				errorChan <- fmt.Errorf("wrong address after invalidation, got %s, want %s", config.ConfigAddress.Hex(), testAddr.Hex())
				return
			}
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(errorChan)

	// Check for any errors
	for err := range errorChan {
		t.Error(err)
	}

	// Final verification
	config, found = provider.GetConfig(testAddr, stateDB, true)
	if !found {
		t.Error("Config should be found after concurrent operations")
	} else if !config.IsActive || config.ConfigAddress != testAddr {
		t.Errorf("Invalid final config state: active=%v, addr=%s", config.IsActive, config.ConfigAddress.Hex())
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

// TestReadConfigAtIndexErrors tests the readConfigAtIndex function for errors
func TestReadConfigAtIndexErrors(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	storage := map[common.Hash]common.Hash{
		common.HexToHash("0x0000000000000000000000000000000000000003"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000100"),
	}
	stateDB := &mockStateDB{storage: storage}

	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Test reading non-existent config
	config, err := provider.readConfigAtIndex(0, stateDB, true)
	if err != nil {
		t.Logf("Expected error when reading non-existent config: %v", err)
	}
	if config.IsActive {
		t.Error("Config should not be active when reading non-existent config")
	}

	// Test invalid events length
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// Set up an invalid config with too many events
	packedData := make([]byte, 32)
	packedData[11] = 1 // isActive = true
	copy(packedData[12:32], common.HexToAddress("0x1234").Bytes())

	storage[configsBaseSlot] = common.BytesToHash(packedData)
	storage[incrementHash(configsBaseSlot)] = common.BigToHash(big.NewInt(256)) // events.length = 256 (too large)

	config, err = provider.readConfigAtIndex(0, stateDB, true)
	if err != nil {
		t.Logf("Expected error when reading config with invalid events length: %v", err)
	}
	if len(config.Events) > int(provider.GetMaxEvents(stateDB, true)) {
		t.Error("Events length should be capped at MaxEvents variable")
	}
}

// TestFindConfigForAddressEdgeCases tests the findConfigForAddress function for edge cases
func TestFindConfigForAddressEdgeCases(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	storage := make(map[common.Hash]common.Hash)
	stateDB := &mockStateDB{storage: storage}

	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	testAddr := common.HexToAddress("0x1234")

	// Test with empty storage
	_, found := provider.findConfigForAddress(testAddr, stateDB, true)
	if found {
		t.Error("Should not find config in empty storage")
	}

	// Test with invalid configs length
	configsSlot := common.BigToHash(big.NewInt(CONFIGS_STORAGE_SLOT))
	storage[configsSlot] = common.BigToHash(big.NewInt(-1)) // Invalid length

	_, found = provider.findConfigForAddress(testAddr, stateDB, true)
	if found {
		t.Error("Should not find config with invalid configs length")
	}

	// Test with nil stateDB
	_, found = provider.findConfigForAddress(testAddr, nil, true)
	if found {
		t.Error("Should not find config with nil stateDB")
	}

	// Test with invalid config data
	storage[configsSlot] = common.BigToHash(big.NewInt(1)) // One config
	configsBaseSlotBytes := crypto.Keccak256(configsSlot[:])
	configsBaseSlot := common.BytesToHash(configsBaseSlotBytes)

	// Set up an invalid config
	packedData := make([]byte, 32)
	packedData[11] = 1 // isActive = true
	copy(packedData[12:32], testAddr.Bytes())

	storage[configsBaseSlot] = common.BytesToHash(packedData)
	// Don't set events length - this should make it invalid

	_, found = provider.findConfigForAddress(testAddr, stateDB, true)
	if found {
		t.Error("Should not find config with invalid data")
	}
}
