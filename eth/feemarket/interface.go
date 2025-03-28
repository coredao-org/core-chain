// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Provider defines the interface for fee monetization services
type Provider interface {
	// GetDenominator returns the denominator used for percentages
	GetDenominator(state FeeMarketStateReader, withCache bool) uint64

	// GetConfig returns configuration for a specific address
	GetConfig(address common.Address, state FeeMarketStateReader, withCache bool) (types.FeeMarketConfig, bool)

	// InvalidateConfig invalidates the cache for a specific address
	InvalidateConfig(address common.Address)

	// InvalidateConstants invalidates the cache for the constants
	InvalidateConstants()

	// CleanCache cleans the cache
	CleanConfigsCache()
}

// FeeMarketStateReader defines the interface for reading the state of the fee market
type FeeMarketStateReader interface {
	// GetState returns the state of the fee market for an address
	GetState(addr common.Address, hash common.Hash) common.Hash
}
