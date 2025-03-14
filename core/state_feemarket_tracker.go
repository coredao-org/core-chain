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
	"github.com/holiman/uint256"
)

// feeMarketTracker is a tracer for tracking the to addresses of top and internal transactions and their cummulative gas used.
// It also tracks addresses to invalidate the fee market cache for.
// It's not part of eth/tracers as it creates import cycles.
type feeMarketTracker struct {
	hooks *tracing.Hooks

	txToAddress         common.Address // to address of the top transaction
	internalTxToAddress common.Address // to address of the internal transaction

	// gasTracker is a map of addresses to the gas used
	gasTracker map[common.Address]*uint256.Int

	// addressesToInvalidateCache is a list of addresses to invalidate the fee market cache for
	addressesToInvalidateCache []common.Address
}

// newFeeMarketTracker creates a new fee market tracker
func newFeeMarketTracker(hooks *tracing.Hooks) (vm.FeeMarketTrackerReader, error) {
	return &feeMarketTracker{
		hooks:      hooks,
		gasTracker: make(map[common.Address]*uint256.Int),
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

	journaledHooks, err := tracing.WrapWithJournal(&wrapped)
	if err != nil {
		return nil, err
	}

	return journaledHooks, nil
}

func (t *feeMarketTracker) GetGasMap() map[common.Address]*uint256.Int {
	return t.gasTracker
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
	if t.gasTracker[t.internalTxToAddress] == nil {
		t.gasTracker[t.internalTxToAddress] = uint256.NewInt(0)
	}
}

func (t *feeMarketTracker) OnExit(depth int, output []byte, gasUsed uint64, err error, reverted bool) {
	if t.hooks != nil && t.hooks.OnExit != nil {
		t.hooks.OnExit(depth, output, gasUsed, err, reverted)
	}

	if reverted || t.internalTxToAddress == (common.Address{}) {
		return
	}

	t.gasTracker[t.internalTxToAddress] = t.gasTracker[t.internalTxToAddress].Add(t.gasTracker[t.internalTxToAddress], uint256.NewInt(gasUsed))

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
	if t.gasTracker[t.txToAddress] == nil {
		t.gasTracker[t.txToAddress] = uint256.NewInt(0)
	}
}

func (t *feeMarketTracker) OnTxEnd(receipt *types.Receipt, err error) {
	if t.hooks != nil && t.hooks.OnTxEnd != nil {
		t.hooks.OnTxEnd(receipt, err)
	}

	if receipt == nil || receipt.Status == types.ReceiptStatusFailed {
		return
	}

	if t.txToAddress == (common.Address{}) {
		return
	}

	t.gasTracker[t.txToAddress] = t.gasTracker[t.txToAddress].Add(t.gasTracker[t.txToAddress], uint256.NewInt(receipt.GasUsed))

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
