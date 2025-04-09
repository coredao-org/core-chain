// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Provider defines the interface for fee monetization services
type Provider interface {
	// GetConstants returns the constants used for fee monetization
	GetConstants(state FeeMarketStateReader, withCache bool, workID *MiningWorkID) types.FeeMarketConstants

	// GetConfig returns configuration for a specific address
	GetConfig(address common.Address, state FeeMarketStateReader, withCache bool, workID *MiningWorkID) (types.FeeMarketConfig, bool)

	// InvalidateConfig invalidates the cache for a specific address
	InvalidateConfig(address common.Address, workID *MiningWorkID)

	// InvalidateConstants invalidates the cache for the constants
	InvalidateConstants(workID *MiningWorkID)

	BeginMining(parent common.Hash, timestamp, attemptNum uint64) MiningWorkID
	CommitMining(workID MiningWorkID)
	AbortMining()

	// Close closes the provider
	Close() error
}

// FeeMarketStateReader defines the interface for reading the state of the fee market
type FeeMarketStateReader interface {
	// GetState returns the state of the fee market for an address
	GetState(addr common.Address, hash common.Hash) common.Hash
}

// BlockChain provides minimal required chain state reading capabilities
type BlockChain interface {
	CurrentHeader() *types.Header
	GetHeader(hash common.Hash, number uint64) *types.Header
	// }

	// // ChainEventSubscriber provides chain event subscription capabilities
	// type ChainEventSubscriber interface {
	FeeMarketSubscribeChainEvent(ch chan<- ChainEvent) event.Subscription
}

// Header contains only the fields needed by feemarket
type Header struct {
	Number     *big.Int
	Hash       common.Hash
	ParentHash common.Hash
}

// ChainEvent is a simplified version of core.ChainEvent
type ChainEvent struct {
	Block *Header
	Hash  common.Hash
}
