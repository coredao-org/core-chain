// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
)

// feeMarketTracker is a tracer for tracking the to addresses of top and internal transactions and their cummulative gas used.
// It also tracks addresses to invalidate the fee market cache for.
// It's not part of eth/tracers as it creates import cycles.
type feeMarketTracker struct {
	hooks *tracing.Hooks

	txToAddress         common.Address // to address of the top transaction
	internalTxToAddress common.Address // to address of the internal transaction

	// gasTracker is a map of addresses to the gas used
	gasTracker map[common.Address]uint64

	// addressesToInvalidateCache is a list of addresses to invalidate the fee market cache for
	addressesToInvalidateCache []common.Address
}

// newFeeMarketTracker creates a new fee market tracker
func newFeeMarketTracker(hooks *tracing.Hooks) (vm.FeeMarketTrackerReader, error) {
	return &feeMarketTracker{
		hooks:      hooks,
		gasTracker: make(map[common.Address]uint64),
	}, nil
}

// Hooks returns the tracer hooks for the fee market
func (t *feeMarketTracker) Hooks() (*tracing.Hooks, error) {
	wrapped := tracing.Hooks{}

	if t.hooks != nil {
		// get a copy of all hooks from the original hooks
		wrapped = *t.hooks
	}

	wrapped.OnTxStart = t.OnTxStart
	wrapped.OnTxEnd = t.OnTxEnd
	wrapped.OnEnter = t.OnEnter
	wrapped.OnExit = t.OnExit
	wrapped.OnLog = t.OnLog

	return &wrapped, nil
}

// GetGasMap returns the gas map for the fee market
func (t *feeMarketTracker) GetGasMap() map[common.Address]uint64 {
	return t.gasTracker
}

// GetAddressesToInvalidateCache returns the addresses to invalidate cache for the fee market
func (t *feeMarketTracker) GetAddressesToInvalidateCache() []common.Address {
	return t.addressesToInvalidateCache
}

func (t *feeMarketTracker) OnEnter(depth int, typ byte, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	if t.hooks != nil && t.hooks.OnEnter != nil {
		t.hooks.OnEnter(depth, typ, from, to, input, gas, value)
	}

	toCopy := to
	if toCopy == (common.Address{}) {
		return
	}

	t.internalTxToAddress = toCopy
	if _, found := t.gasTracker[t.internalTxToAddress]; !found {
		t.gasTracker[t.internalTxToAddress] = 0
	}
}

func (t *feeMarketTracker) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if t.hooks != nil && t.hooks.OnExit != nil {
		t.hooks.OnExit(depth, output, gasUsed, err, reverted)
	}

	// TODO: do we care if the tx reverted? the gas cost happened either way
	if reverted || t.internalTxToAddress == (common.Address{}) {
		return
	}

	t.gasTracker[t.internalTxToAddress] += gasUsed

	t.internalTxToAddress = (common.Address{})
}

func (t *feeMarketTracker) OnTxStart(vm *tracing.VMContext, tx *types.Transaction, from common.Address) {
	if t.hooks != nil && t.hooks.OnTxStart != nil {
		t.hooks.OnTxStart(vm, tx, from)
	}

	toCopy := tx.To()
	if toCopy == nil || *toCopy == (common.Address{}) {
		return
	}

	t.txToAddress = *toCopy
	if _, found := t.gasTracker[t.txToAddress]; !found {
		t.gasTracker[t.txToAddress] = 0
	}
}

func (t *feeMarketTracker) OnTxEnd(receipt *types.Receipt, err error) {
	if t.hooks != nil && t.hooks.OnTxEnd != nil {
		t.hooks.OnTxEnd(receipt, err)
	}

	// Error happened during tx validation.
	if err != nil || receipt == nil {
		return
	}

	// We want to track the gas used by the tx even if it failed.
	if t.txToAddress == (common.Address{}) {
		return
	}

	// As this is the actual TX gas used, we don't need to add it on top of the internal tx gas used
	t.gasTracker[t.txToAddress] += receipt.GasUsed

	// TODO: shall we invalidate gas use for other addresses too? or mark this address as root level?

	t.txToAddress = (common.Address{})
}

func (t *feeMarketTracker) OnLog(l *types.Log) {
	if t.hooks != nil && t.hooks.OnLog != nil {
		t.hooks.OnLog(l)
	}

	if l.Address != common.HexToAddress(systemcontracts.FeeMarketContract) {
		return
	}

	// TODO: add logic to invalidate the fee market cache for the address
}
