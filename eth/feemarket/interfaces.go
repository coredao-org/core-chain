// Package feemarket provides implementations for fee market monetization.
package feemarket

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// ProviderCache defines the interface for handling caching of fee market data
type ProviderCache interface {
	// InvalidateConfig invalidates the cache for a specific address
	InvalidateConfig(address common.Address, workID *MiningWorkID)

	// InvalidateConstants invalidates the cache for the constants
	InvalidateConstants(workID *MiningWorkID)

	// BeginMining begins a new mining session,
	// multiple mining sessions can be active at the same time for the same block
	BeginMining(parent common.Hash, timestamp, attemptNum uint64) MiningWorkID

	// CommitMining commits the only the winning mining session entries
	CommitMining(workID MiningWorkID)

	// AbortMining cleans up all temp caches for this mining block
	AbortMining()

	// Close closes the cache manager
	Close() error
}

// Provider defines the interface for fee monetization services
type Provider interface {
	ProviderCache

	// GetConstants returns the constants used for fee monetization
	GetConstants(state StateReader, blockNumber uint64, withCache bool, workID *MiningWorkID) types.FeeMarketConstants

	// GetConfig returns configuration for a specific address
	GetConfig(address common.Address, state StateReader, blockNumber uint64, withCache bool, workID *MiningWorkID) (types.FeeMarketConfig, bool)
}

// StateReader defines the interface for reading the state of the fee market
type StateReader interface {
	// GetState returns the state of the fee market for an address
	GetState(addr common.Address, hash common.Hash) common.Hash
}

// BlockChain provides minimal required chain state reading capabilities
type BlockChain interface {
	CurrentHeader() *types.Header
	GetHeader(hash common.Hash, number uint64) *types.Header

	// FeeMarketSubscribeChainHeadEvent provides chain event subscription capabilities
	FeeMarketSubscribeChainHeadEvent(ch chan<- ChainHeadEvent) event.Subscription
}

// Header contains only the fields needed by feemarket
type Header struct {
	Number     *big.Int
	Hash       common.Hash
	ParentHash common.Hash
}

// ChainEvent is a simplified version of core.ChainEvent
type ChainHeadEvent struct {
	Block *Header
	Hash  common.Hash
}
