// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// FeeMarket represents the fee market integration which is used to get the fee market config for an address.
type FeeMarket struct {
	// provider is the reader for the FeeMarketContract storage layout
	provider Provider

	// contractAddress is the address of the FeeMarketContract
	contractAddress common.Address
}

// NewFeeMarket creates a new fee market integration using storage access
func NewFeeMarket(bc BlockChain) (*FeeMarket, error) {
	feeMarketContractAddress := common.HexToAddress(systemcontracts.FeeMarketContract)
	provider, err := NewStorageProvider(feeMarketContractAddress, bc)
	if err != nil {
		return nil, err
	}

	return &FeeMarket{
		provider:        provider,
		contractAddress: feeMarketContractAddress,
	}, nil
}

func (fm *FeeMarket) Close() error {
	return fm.provider.Close()
}

// GetConfig gets the fee market config for an address
func (fm *FeeMarket) GetConfig(address common.Address, state FeeMarketStateReader, withCache bool, workID *MiningWorkID) (types.FeeMarketConfig, bool) {
	return fm.provider.GetConfig(address, state, withCache, workID)
}

// HandleCacheInvalidationEvent handles cache invalidation events
func (fm *FeeMarket) HandleCacheInvalidationEvent(eventLog *types.Log, workID *MiningWorkID) bool {
	// If the event is from the FeeMarketContract
	if eventLog.Address == fm.contractAddress {
		// Check if the event is a ConfigUpdated event
		id := common.BytesToHash(crypto.Keccak256([]byte("ConfigUpdated(address,uint256,uint256)")))
		if eventLog.Topics[0] == id && len(eventLog.Topics) > 1 {
			// Get config address from event.topics[1]
			configAddress := common.HexToAddress(eventLog.Topics[1].Hex())
			// Invalidate the config for the address
			fm.provider.InvalidateConfig(configAddress, workID)
			return true
		}

		// Check if the event is a ConstantUpdated event
		id = common.BytesToHash(crypto.Keccak256([]byte("ConstantUpdated()")))
		if eventLog.Topics[0] == id {
			fm.provider.InvalidateConstants(workID)
			return true
		}
	}
	return false
}

// GetDenominator returns the denominator used for percentages
func (fm *FeeMarket) GetDenominator(state FeeMarketStateReader, withCache bool, workID *MiningWorkID) uint64 {
	return fm.provider.GetConstants(state, withCache, workID).Denominator
}

// CleanCache cleans the cache
func (fm *FeeMarket) BeginMining(parent common.Hash, timestamp, attemptNum uint64) MiningWorkID {
	return fm.provider.BeginMining(parent, timestamp, attemptNum)
}
func (fm *FeeMarket) CommitMining(workID MiningWorkID) {
	fm.provider.CommitMining(workID)
}
func (fm *FeeMarket) AbortMining() {
	fm.provider.AbortMining()
}
