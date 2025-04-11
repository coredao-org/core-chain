// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
)

// FeeMarket represents the fee market integration which is used to get the fee market config for an address.
type FeeMarket struct {
	// provider is the reader for the FeeMarketContract storage layout
	provider Provider

	// contractAddress is the address of the FeeMarketContract
	contractAddress common.Address
}

// NewFeeMarket creates a new fee market integration using storage access
func NewFeeMarket() (*FeeMarket, error) {
	feeMarketContractAddress := common.HexToAddress(systemcontracts.FeeMarketContract)
	provider, err := NewStorageProvider(feeMarketContractAddress)
	if err != nil {
		return nil, err
	}

	return &FeeMarket{
		provider:        provider,
		contractAddress: feeMarketContractAddress,
	}, nil
}

// GetConfig gets the fee market config for an address
func (fm *FeeMarket) GetConfig(address common.Address, state StateReader) (types.FeeMarketConfig, bool) {
	return fm.provider.GetConfig(address, state)
}

// GetDenominator returns the denominator used for percentages
func (fm *FeeMarket) GetDenominator(state StateReader) uint64 {
	return fm.provider.GetConstants(state).Denominator
}
