package feemarket

import (
	"fmt"
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
func TestStorageProviderRetrieveFromCache(t *testing.T) {
	testAddr := common.HexToAddress("0x01")
	contractAddr := common.HexToAddress("0x1234")

	provider, err := NewStorageProvider(contractAddr)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Add a test config directly to the cache
	testConfig := types.FeeMarketConfig{
		IsActive:      true,
		ConfigAddress: testAddr,
	}
	provider.configCache[testAddr] = testConfig

	// Test GetConfig with cached value
	_, found := provider.GetConfig(testAddr, nil)
	if !found {
		t.Errorf("Expected to find config for address %s but not found", testAddr.Hex())
	}

	// if config.ConfigRate.Cmp(testConfig.ConfigRate) != 0 {
	// 	t.Errorf("Expected rate %v, got %v", 1000, config.ConfigRate)
	// }

	// Test invalidating and fetching again (should fail as we didn't set up the storage)
	provider.InvalidateConfig(testAddr)
	_, found = provider.GetConfig(testAddr, nil)
	if found {
		t.Errorf("Expected config to not be found after invalidation")
	}

	// Test CleanCache
	provider.CleanCache()
	if len(provider.configCache) != 0 {
		t.Errorf("Expected configCache to be empty after ReloadConfigs")
	}
}

func TestStorageLayoutParsing(t *testing.T) {
	contractAddr := common.HexToAddress("0x3d2316663295d2cf0b4f101050391b13dfff5fdf")
	storage := map[common.Hash]common.Hash{
		common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000005"),
		common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		// eventsBaseSlot = eventSlot 0 (event 0) (rewards.length)
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e0"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// eventSlot 0  -> eventSigSlot (event 0) (EventSignature)
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e1"): common.HexToHash("0x3fb5c1cb9d57cc981b075ac270f9215e697bc33dacd5ce87319656ebf8fc7b92"),
		// eventSlot 0  -> gasSlot (event 0) (Gas)
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e2"): common.HexToHash("0x00000000000000000000000000000000000000000000000000000000000186a0"),
		// eventSlot (event 1)
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e3"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e4"): common.HexToHash("0xd09de08ab1a974aadf0a76e6f99a2ec20e431f22bbc101a6c3f718e53646ed8d"),
		common.HexToHash("0x1ab0c6948a275349ae45a06aad66a8bd65ac18074615d53676c09b67809099e5"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000030d40"),
		// configsBaseSlot (events and functionSignatures lengths)
		common.HexToHash("0x405787fa12a823e0f2b7631cc41b3ba8828b3321ca811111fa75cd3aa3bb5ace"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
		// configsBaseSlot + 2 (packed isActive and configAddress)
		common.HexToHash("0x405787fa12a823e0f2b7631cc41b3ba8828b3321ca811111fa75cd3aa3bb5ad0"): common.HexToHash("0x0000000000000000000000013d2316663295d2cf0b4f101050391b13dfff5fdf"),
		// eventSlot 0 -> rewardsBaseSlot (reward 0) (rewardAddr + rewardPercentage)
		common.HexToHash("0xc7c06de7e7d060da46f9721814db6bb8a757e1990dfeffbc755bf904891267a5"): common.HexToHash("0x0000000000000000000023288f10d3a6283672ecfaeea0377d460bded489ec44"),
		// eventSlot 0 -> rewardsBaseSlot (reward 1) (rewardAddr + rewardPercentage)
		common.HexToHash("0xc7c06de7e7d060da46f9721814db6bb8a757e1990dfeffbc755bf904891267a6"): common.HexToHash("0x0000000000000000000003e80000000000000000000000000000000000000789"),
	}

	provider, err := NewStorageProvider(contractAddr)
	if err != nil {
		t.Fatalf("Failed to create storage provider: %v", err)
	}

	stateDB := &mockStateDB{storage: storage}
	config, found := provider.GetConfig(contractAddr, stateDB)
	if !found {
		t.Errorf("Config not found")
	}

	if !config.IsValidConfig(nil) {
		t.Errorf("Config is not valid")
	}

	fmt.Println("config:", config)
}
