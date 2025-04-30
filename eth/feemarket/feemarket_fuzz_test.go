//go:build go1.18
// +build go1.18

package feemarket

import (
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// FuzzFeeMarketConfigStorage fuzzes GetActiveConfig and IsValidConfig for random storage layouts.
func FuzzFeeMarketConfigStorage(f *testing.F) {
	rand.Seed(time.Now().UnixNano())
	f.Add(uint64(1000), uint8(2), uint8(2), uint32(1000000)) // seed corpus

	f.Fuzz(func(t *testing.T, addrSeed uint64, maxEvents uint8, maxRewards uint8, maxGas uint32) {
		t.Parallel()
		storage := map[common.Hash]common.Hash{}
		constants := types.FeeMarketConstants{
			MaxEvents:  maxEvents%10 + 1, // avoid zero
			MaxRewards: maxRewards%10 + 1,
			MaxGas:     maxGas%1000000 + 1,
		}
		writeConstants(storage, constants)

		addr := common.BigToAddress(big.NewInt(int64(addrSeed)))
		_ = writeRandomConfiguration(storage, addr, constants)

		stateDB := &mockStateDB{storage: storage}
		fm := NewFeeMarket()

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic: %v", r)
			}
		}()

		config, _, found := fm.GetActiveConfig(addr, stateDB)
		if found {
			// Should always be valid or error gracefully
			valid, err := config.IsValidConfig(constants, 10000)
			if err != nil && valid {
				t.Errorf("IsValidConfig returned valid but error: %v", err)
			}
		}
	})
}
