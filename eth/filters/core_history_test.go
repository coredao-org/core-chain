// Copyright 2014 The go-ethereum Authors
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

package filters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/filtermaps"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

// Historical samples occupy consecutive fixture positions while retaining their
// original headers, receipts and log metadata. This exercises index contents,
// not a full mainnet synchronization or mainnet block-range layout.
type coreHistoryBackend struct {
	*testBackend
	head     uint64
	receipts map[common.Hash]types.Receipts
	reads    atomic.Int64
}

func (b *coreHistoryBackend) CurrentView() *filtermaps.ChainView {
	return filtermaps.NewChainView(b, b.head, b.GetCanonicalHash(b.head))
}

func (b *coreHistoryBackend) GetRawReceipts(hash common.Hash, _ uint64) types.Receipts {
	return b.receipts[hash]
}

func (b *coreHistoryBackend) GetReceiptsByHash(hash common.Hash) types.Receipts {
	return b.receipts[hash]
}

func (b *coreHistoryBackend) NewMatcherBackend() filtermaps.MatcherBackend {
	return &coreHistoryMatcher{b.fm.NewMatcherBackend(), &b.reads}
}

type coreHistoryMatcher struct {
	filtermaps.MatcherBackend
	reads *atomic.Int64
}

func (m *coreHistoryMatcher) GetFilterMapRows(ctx context.Context, maps []uint32, row uint32, base bool) ([]filtermaps.FilterRow, error) {
	m.reads.Add(1)
	return m.MatcherBackend.GetFilterMapRows(ctx, maps, row, base)
}

// TestCoreHistoryFilterMaps replays data captured by capture-core-log-fixture.py.
// It is opt-in because the complete public mainnet fixture is intentionally not
// downloaded during ordinary unit tests.
func TestCoreHistoryFilterMaps(t *testing.T) {
	path := os.Getenv("CORE_FILTERMAPS_FIXTURE")
	if path == "" {
		t.Skip("set CORE_FILTERMAPS_FIXTURE to a captured Core history fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Blocks []struct {
			Number   uint64         `json:"number"`
			RawBlock hexutil.Bytes  `json:"rawBlock"`
			Receipts types.Receipts `json:"receipts"`
			Logs     []*types.Log   `json:"logs"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Blocks) < 2 || fixture.Blocks[0].Number != 0 {
		t.Fatal("fixture must start with genesis and contain historical samples")
	}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	backend := &coreHistoryBackend{testBackend: &testBackend{db: db}, head: uint64(len(fixture.Blocks) - 1), receipts: make(map[common.Hash]types.Receipts)}
	var logCount int
	for i, item := range fixture.Blocks {
		var block types.Block
		if err := rlp.DecodeBytes(item.RawBlock, &block); err != nil {
			t.Fatal(err)
		}
		if block.NumberU64() != item.Number || types.DeriveSha(item.Receipts, trie.NewStackTrie(nil)) != block.ReceiptHash() {
			t.Fatalf("block %d: invalid receipt root or height", item.Number)
		}
		for _, entry := range item.Logs {
			if entry.BlockTimestamp != block.Time() || entry.BlockHash != block.Hash() || entry.BlockNumber != item.Number {
				t.Fatalf("block %d: invalid log metadata", item.Number)
			}
		}
		logCount += len(item.Logs)
		rawdb.WriteBlock(db, &block)
		rawdb.WriteReceipts(db, block.Hash(), item.Number, item.Receipts)
		rawdb.WriteCanonicalHash(db, block.Hash(), uint64(i))
		rawdb.WriteHeadBlockHash(db, block.Hash())
		backend.receipts[block.Hash()] = item.Receipts
	}

	var total int
	for _, mode := range []struct {
		name     string
		history  uint64
		disabled bool
	}{{"indexed", 0, false}, {"reopened", 0, false}, {"limited", 100, false}, {"disabled", 0, true}} {
		t.Run(mode.name, func(t *testing.T) {
			fm, err := filtermaps.NewFilterMaps(db, backend.CurrentView(), 0, 0, filtermaps.DefaultParams, filtermaps.Config{History: mode.history, Disabled: mode.disabled})
			if err != nil {
				t.Fatal(err)
			}
			backend.fm = fm
			fm.Start()
			defer fm.Stop()
			fm.WaitIdle()
			backend.reads.Store(0)
			sys := NewFilterSystem(backend, Config{})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			comparisons := 0
			check := func(first, last int, addresses []common.Address, topics [][]common.Hash) {
				t.Helper()
				var want []*types.Log
				for _, item := range fixture.Blocks[first : last+1] {
					for _, entry := range item.Logs {
						if len(addresses) != 0 && !slices.Contains(addresses, entry.Address) || len(entry.Topics) < len(topics) {
							continue
						}
						matches := true
						for i, choices := range topics {
							if len(choices) != 0 && !slices.Contains(choices, entry.Topics[i]) {
								matches = false
								break
							}
						}
						if matches {
							want = append(want, entry)
						}
					}
				}
				got, err := sys.NewRangeFilter(int64(first), int64(last), addresses, topics, false).Logs(ctx)
				if err != nil || len(got) != len(want) || len(got) != 0 && !reflect.DeepEqual(got, want) {
					t.Fatalf("fixture positions %d..%d: got %d logs, want %d, error %v", first, last, len(got), len(want), err)
				}
				comparisons++
			}
			check(0, int(backend.head), nil, nil)
			for i, item := range fixture.Blocks {
				check(i, i, nil, nil)
				if len(item.Logs) == 0 {
					continue
				}
				entry := item.Logs[0]
				check(max(0, i-3), min(int(backend.head), i+3), []common.Address{entry.Address}, nil)
				if len(entry.Topics) != 0 {
					check(max(0, i-4), min(int(backend.head), i+4), nil, [][]common.Hash{{entry.Topics[0]}})
				}
				if len(entry.Topics) > 1 {
					check(max(0, i-5), min(int(backend.head), i+5), []common.Address{entry.Address}, [][]common.Hash{nil, {entry.Topics[1]}})
				}
			}
			if !mode.disabled && backend.reads.Load() == 0 {
				t.Fatal("queries did not read filtermaps index rows")
			}
			if mode.disabled && backend.reads.Load() != 0 {
				t.Fatal("disabled mode read filtermaps index rows")
			}
			total += comparisons
			t.Logf("comparisons=%d indexRowReads=%d", comparisons, backend.reads.Load())
		})
	}
	sum := sha256.Sum256(data)
	t.Logf("blocks=%d logs=%d comparisons=%d fixtureSha256=%s", len(fixture.Blocks), logCount, total, hex.EncodeToString(sum[:]))
}
