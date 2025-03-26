// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

const (
	// DenominatorValue is the denominator used for percentages (10000 = 100.00%)
	DenominatorValue = uint64(10000)
)

// FeeMarket represents the fee market integration which is used to get the fee market config for an address.
type FeeMarket struct {
	provider Provider
}

// NewFeeMarket creates a new fee market integration using storage access
func NewFeeMarket() (*FeeMarket, error) {
	feeMarketContractAddress := common.HexToAddress(systemcontracts.FeeMarketContract)
	provider, err := NewStorageProvider(feeMarketContractAddress)
	if err != nil {
		return nil, err
	}

	return &FeeMarket{
		provider: provider,
	}, nil
}

// GetConfig gets the fee market config for an address
func (fm *FeeMarket) GetConfig(address common.Address, state FeeMarketStateReader) (types.FeeMarketConfig, bool) {
	return fm.provider.GetConfig(address, state)
}

// InvalidateConfig invalidates the cache for a specific address
func (fm *FeeMarket) InvalidateConfig(address common.Address) {
	fm.provider.InvalidateConfig(address)
}

// GetDenominator returns the denominator used for percentages
func (fm *FeeMarket) GetDenominator(state FeeMarketStateReader) uint64 {
	return fm.provider.GetDenominator(state)
}

// CleanCache cleans the cache
func (fm *FeeMarket) CleanCache() {
	fm.provider.CleanCache()
}

// GetDenominatorBig returns the denominator as a uint256.Int
