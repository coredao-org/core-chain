package feemarket

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

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
		ConfigRate:     uint256.NewInt(1000),
		UserConfigRate: uint256.NewInt(300),
		IsActive:       true,
		ConfigAddress:  testAddr,
		Rewards: []types.FeeMarketReward{
			{
				RewardAddress:    common.HexToAddress("0x02"),
				RewardPercentage: uint256.NewInt(700),
			},
		},
	}
	provider.configCache[testAddr] = testConfig

	// Test GetConfig with cached value
	config, found := provider.GetConfig(testAddr, nil)
	if !found {
		t.Errorf("Expected to find config for address %s but not found", testAddr.Hex())
	}

	if config.ConfigRate.Cmp(testConfig.ConfigRate) != 0 {
		t.Errorf("Expected rate %v, got %v", 1000, config.ConfigRate)
	}

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
