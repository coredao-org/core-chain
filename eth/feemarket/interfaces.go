// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Provider defines the interface for fee monetization services
type Provider interface {
	// GetConstants returns the constants used for fee monetization
	GetConstants(state StateReader) types.FeeMarketConstants

	// GetConfig returns configuration for a specific address
	GetConfig(address common.Address, state StateReader) (types.FeeMarketConfig, bool)
}

// StateReader defines the interface for reading the state of the fee market
type StateReader interface {
	// GetState returns the state of the fee market for an address
	GetState(addr common.Address, hash common.Hash) common.Hash
}
