package feemarket

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type mockStateDB struct {
	storage map[common.Hash]common.Hash
}

func (m *mockStateDB) GetState(addr common.Address, key common.Hash) common.Hash {
	return m.storage[key]
}

// TestStorageProviderRetrieveFromCache tests basic functionality of StorageProvider
func TestCache(t *testing.T) {
	testAddr := common.HexToAddress("0x01")
	contractAddr := common.HexToAddress("0x1234")

	stateDB := &mockStateDB{}
	provider, err := NewStorageProvider(contractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	provider.EnableCache()

	// Add a test config directly to the cache
	testConfig := types.FeeMarketConfig{
		IsActive:      true,
		ConfigAddress: testAddr,
	}
	provider.configCache[testAddr] = testConfig

	// Test GetConfig with cached value
	_, found := provider.GetConfig(testAddr, stateDB)
	if !found {
		t.Errorf("Expected to find config for address %s but not found", testAddr.Hex())
	}

	// if config.ConfigRate.Cmp(testConfig.ConfigRate) != 0 {
	// 	t.Errorf("Expected rate %v, got %v", 1000, config.ConfigRate)
	// }

	// Test invalidating and fetching again (should fail as we didn't set up the storage)
	provider.InvalidateConfig(testAddr)
	_, found = provider.GetConfig(testAddr, stateDB)
	if found {
		t.Errorf("Expected config to not be found after invalidation")
	}

	// Test CleanCache
	provider.CleanCache()
	if len(provider.configCache) != 0 {
		t.Errorf("Expected configCache to be empty after ReloadConfigs")
	}
}

func TestReadingOfConstants(t *testing.T) {
	configurationContractAddr := common.HexToAddress("0x0000000000000000000000000000000000001016")
	storage := map[common.Hash]common.Hash{

		common.HexToHash("0x0000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"),
		common.HexToHash("0x0000000000000000000000000000000000000002"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000005"),
		common.HexToHash("0x0000000000000000000000000000000000000003"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000006"),
		common.HexToHash("0x0000000000000000000000000000000000000004"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000007"),
		common.HexToHash("0x0000000000000000000000000000000000000005"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000f4240"),
	}

	stateDB := &mockStateDB{storage: storage}

	provider, err := NewStorageProvider(configurationContractAddr)
	if err != nil {
		t.Fatalf("Failed to create storage provider: %v", err)
	}

	provider.EnableCache()

	checkInitialValues := func(st *mockStateDB) {
		denominator := provider.GetDenominator(st)
		if denominator != 10000 {
			t.Errorf("Expected denominator to be 10000, got %d", denominator)
		}

		maxRewards := provider.GetMaxRewards(st)
		if maxRewards != 5 {
			t.Errorf("Expected maxRewards to be 5, got %d", maxRewards)
		}

		maxEvents := provider.GetMaxEvents(st)
		if maxEvents != 6 {
			t.Errorf("Expected maxEvents to be 6, got %d", maxEvents)
		}

		maxFunctionSignatures := provider.GetMaxFunctionSignatures(st)
		if maxFunctionSignatures != 7 {
			t.Errorf("Expected maxFunctionSignatures to be 7, got %d", maxFunctionSignatures)
		}

		maxGas := provider.GetMaxGas(st)
		if maxGas != 1000000 {
			t.Errorf("Expected maxGas to be 1000000, got %d", maxGas)
		}
	}
	checkInitialValues(stateDB)

	updatedStorage := map[common.Hash]common.Hash{
		common.HexToHash("0x0000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		common.HexToHash("0x0000000000000000000000000000000000000002"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		common.HexToHash("0x0000000000000000000000000000000000000003"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000003"),
		common.HexToHash("0x0000000000000000000000000000000000000004"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000004"),
		common.HexToHash("0x0000000000000000000000000000000000000005"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000005"),
	}
	stateDB.storage = updatedStorage

	// Check that provider returns the cached values if not invalidated
	checkInitialValues(stateDB)

	// Invalidate the config
	provider.InvalidateConstants()

	// Check that provider returns the updated values
	denominator := provider.GetDenominator(stateDB)
	if denominator != 1 {
		t.Errorf("Expected denominator to be 1, got %d", denominator)
	}

	maxRewards := provider.GetMaxRewards(stateDB)
	if maxRewards != 2 {
		t.Errorf("Expected maxRewards to be 2, got %d", maxRewards)
	}

	maxEvents := provider.GetMaxEvents(stateDB)
	if maxEvents != 3 {
		t.Errorf("Expected maxEvents to be 3, got %d", maxEvents)
	}

	maxFunctionSignatures := provider.GetMaxFunctionSignatures(stateDB)
	if maxFunctionSignatures != 4 {
		t.Errorf("Expected maxFunctionSignatures to be 4, got %d", maxFunctionSignatures)
	}

	maxGas := provider.GetMaxGas(stateDB)
	if maxGas != 5 {
		t.Errorf("Expected maxGas to be 5, got %d", maxGas)
	}
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
	config, found := provider.GetConfig(contractAddr1, stateDB)
	if !found {
		t.Errorf("Config not found")
	}

	if !config.IsValidConfig(provider.GetDenominator(stateDB), provider.GetMaxGas(stateDB), provider.GetMaxEvents(stateDB), provider.GetMaxRewards(stateDB)) {
		t.Errorf("Config is not valid")
	}

	config, found = provider.GetConfig(contractAddr2, stateDB)
	if !found {
		t.Errorf("Config not found")
	}

	if !config.IsValidConfig(provider.GetDenominator(stateDB), provider.GetMaxGas(stateDB), provider.GetMaxEvents(stateDB), provider.GetMaxRewards(stateDB)) {
		t.Errorf("Config is not valid")
	}

	// TODO: validate the config values as well
}
