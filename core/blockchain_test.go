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

package core

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/tracers/logger"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/holiman/uint256"
)

// So we can deterministically seed different blockchains
var (
	canonicalSeed = 1
	forkSeed      = 2

	TestTriesInMemory = 128
)

// newCanonical creates a chain database, and injects a deterministic canonical
// chain. Depending on the full flag, it creates either a full block chain or a
// header only chain. The database and genesis specification for block generation
// are also returned in case more test blocks are needed later.
func newCanonical(engine consensus.Engine, n int, full bool, scheme string) (ethdb.Database, *Genesis, *BlockChain, error) {
	var (
		genesis = &Genesis{
			BaseFee: big.NewInt(params.InitialBaseFee),
			Config:  params.AllEthashProtocolChanges,
		}
	)
	// Initialize a fresh chain with only a genesis block
	var ops []BlockChainOption
	blockchain, _ := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil, ops...)
	// Create and inject the requested chain
	if n == 0 {
		return rawdb.NewMemoryDatabase(), genesis, blockchain, nil
	}
	if full {
		// Full block-chain requested
		genDb, blocks := makeBlockChainWithGenesis(genesis, n, engine, canonicalSeed)
		_, err := blockchain.InsertChain(blocks)
		return genDb, genesis, blockchain, err
	}
	// Header-only chain requested
	genDb, headers := makeHeaderChainWithGenesis(genesis, n, engine, canonicalSeed)
	_, err := blockchain.InsertHeaderChain(headers)
	return genDb, genesis, blockchain, err
}

func newGwei(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(params.GWei))
}

// Test fork of length N starting from block i
func testFork(t *testing.T, blockchain *BlockChain, i, n int, full bool, comparator func(td1, td2 *big.Int), scheme string) {
	// Copy old chain up to #i into a new db
	genDb, _, blockchain2, err := newCanonical(ethash.NewFaker(), i, full, scheme)
	if err != nil {
		t.Fatal("could not make new canonical in testFork", err)
	}
	defer blockchain2.Stop()

	// Assert the chains have the same header/block at #i
	var hash1, hash2 common.Hash
	if full {
		hash1 = blockchain.GetBlockByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetBlockByNumber(uint64(i)).Hash()
	} else {
		hash1 = blockchain.GetHeaderByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetHeaderByNumber(uint64(i)).Hash()
	}
	if hash1 != hash2 {
		t.Errorf("chain content mismatch at %d: have hash %v, want hash %v", i, hash2, hash1)
	}
	// Extend the newly created chain
	var (
		blockChainB  []*types.Block
		headerChainB []*types.Header
	)
	if full {
		blockChainB = makeBlockChain(blockchain2.chainConfig, blockchain2.GetBlockByHash(blockchain2.CurrentBlock().Hash()), n, ethash.NewFaker(), genDb, forkSeed)
		if _, err := blockchain2.InsertChain(blockChainB); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
	} else {
		headerChainB = makeHeaderChain(blockchain2.chainConfig, blockchain2.CurrentHeader(), n, ethash.NewFaker(), genDb, forkSeed)
		if _, err := blockchain2.InsertHeaderChain(headerChainB); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
	}
	// Sanity check that the forked chain can be imported into the original
	var tdPre, tdPost *big.Int

	if full {
		cur := blockchain.CurrentBlock()
		tdPre = blockchain.GetTd(cur.Hash(), cur.Number.Uint64())
		if err := testBlockChainImport(blockChainB, blockchain); err != nil {
			t.Fatalf("failed to import forked block chain: %v", err)
		}
		last := blockChainB[len(blockChainB)-1]
		tdPost = blockchain.GetTd(last.Hash(), last.NumberU64())
	} else {
		cur := blockchain.CurrentHeader()
		tdPre = blockchain.GetTd(cur.Hash(), cur.Number.Uint64())
		if err := testHeaderChainImport(headerChainB, blockchain); err != nil {
			t.Fatalf("failed to import forked header chain: %v", err)
		}
		last := headerChainB[len(headerChainB)-1]
		tdPost = blockchain.GetTd(last.Hash(), last.Number.Uint64())
	}
	// Compare the total difficulties of the chains
	comparator(tdPre, tdPost)
}

// testBlockChainImport tries to process a chain of blocks, writing them into
// the database if successful.
func testBlockChainImport(chain types.Blocks, blockchain *BlockChain) error {
	for _, block := range chain {
		// Try and process the block
		err := blockchain.engine.VerifyHeader(blockchain, block.Header())
		if err == nil {
			err = blockchain.validator.ValidateBody(block)
		}
		if err != nil {
			if err == ErrKnownBlock {
				continue
			}
			return err
		}
		statedb, err := state.New(blockchain.GetBlockByHash(block.ParentHash()).Root(), blockchain.stateCache, nil)
		if err != nil {
			return err
		}
		statedb.SetExpectedStateRoot(block.Root())
		statedb, receipts, _, usedGas, err := blockchain.processor.Process(block, statedb, vm.Config{})
		if err != nil {
			blockchain.reportBlock(block, receipts, err)
			return err
		}
		err = blockchain.validator.ValidateState(block, statedb, receipts, usedGas)
		if err != nil {
			blockchain.reportBlock(block, receipts, err)
			return err
		}

		blockchain.chainmu.MustLock()
		rawdb.WriteTd(blockchain.db, block.Hash(), block.NumberU64(), new(big.Int).Add(block.Difficulty(), blockchain.GetTd(block.ParentHash(), block.NumberU64()-1)))
		rawdb.WriteBlock(blockchain.db, block)
		statedb.Commit(block.NumberU64(), false)
		blockchain.chainmu.Unlock()
	}
	return nil
}

// testHeaderChainImport tries to process a chain of header, writing them into
// the database if successful.
func testHeaderChainImport(chain []*types.Header, blockchain *BlockChain) error {
	for _, header := range chain {
		// Try and validate the header
		if err := blockchain.engine.VerifyHeader(blockchain, header); err != nil {
			return err
		}
		// Manually insert the header into the database, but don't reorganise (allows subsequent testing)
		blockchain.chainmu.MustLock()
		rawdb.WriteTd(blockchain.db, header.Hash(), header.Number.Uint64(), new(big.Int).Add(header.Difficulty, blockchain.GetTd(header.ParentHash, header.Number.Uint64()-1)))
		rawdb.WriteHeader(blockchain.db, header)
		blockchain.chainmu.Unlock()
	}
	return nil
}

func TestLastBlock(t *testing.T) {
	testLastBlock(t, rawdb.HashScheme)
	testLastBlock(t, rawdb.PathScheme)
}

func testLastBlock(t *testing.T, scheme string) {
	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, true, scheme)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	blocks := makeBlockChain(blockchain.chainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 1, ethash.NewFullFaker(), genDb, 0)
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}
	if blocks[len(blocks)-1].Hash() != rawdb.ReadHeadBlockHash(blockchain.db) {
		t.Fatalf("Write/Get HeadBlockHash failed")
	}
}

// Test inserts the blocks/headers after the fork choice rule is changed.
// The chain is reorged to whatever specified.
func testInsertAfterMerge(t *testing.T, blockchain *BlockChain, i, n int, full bool, scheme string) {
	// Copy old chain up to #i into a new db
	genDb, _, blockchain2, err := newCanonical(ethash.NewFaker(), i, full, scheme)
	if err != nil {
		t.Fatal("could not make new canonical in testFork", err)
	}
	defer blockchain2.Stop()

	// Assert the chains have the same header/block at #i
	var hash1, hash2 common.Hash
	if full {
		hash1 = blockchain.GetBlockByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetBlockByNumber(uint64(i)).Hash()
	} else {
		hash1 = blockchain.GetHeaderByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetHeaderByNumber(uint64(i)).Hash()
	}
	if hash1 != hash2 {
		t.Errorf("chain content mismatch at %d: have hash %v, want hash %v", i, hash2, hash1)
	}

	// Extend the newly created chain
	if full {
		blockChainB := makeBlockChain(blockchain2.chainConfig, blockchain2.GetBlockByHash(blockchain2.CurrentBlock().Hash()), n, ethash.NewFaker(), genDb, forkSeed)
		if _, err := blockchain2.InsertChain(blockChainB); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
		if blockchain2.CurrentBlock().Number.Uint64() != blockChainB[len(blockChainB)-1].NumberU64() {
			t.Fatalf("failed to reorg to the given chain")
		}
		if blockchain2.CurrentBlock().Hash() != blockChainB[len(blockChainB)-1].Hash() {
			t.Fatalf("failed to reorg to the given chain")
		}
	} else {
		headerChainB := makeHeaderChain(blockchain2.chainConfig, blockchain2.CurrentHeader(), n, ethash.NewFaker(), genDb, forkSeed)
		if _, err := blockchain2.InsertHeaderChain(headerChainB); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
		if blockchain2.CurrentHeader().Number.Uint64() != headerChainB[len(headerChainB)-1].Number.Uint64() {
			t.Fatalf("failed to reorg to the given chain")
		}
		if blockchain2.CurrentHeader().Hash() != headerChainB[len(headerChainB)-1].Hash() {
			t.Fatalf("failed to reorg to the given chain")
		}
	}
}

// Tests that given a starting canonical chain of a given size, it can be extended
// with various length chains.
func TestExtendCanonicalHeaders(t *testing.T) {
	testExtendCanonical(t, false, rawdb.HashScheme)
	testExtendCanonical(t, false, rawdb.PathScheme)
}

func TestExtendCanonicalBlocks(t *testing.T) {
	testExtendCanonical(t, true, rawdb.HashScheme)
	testExtendCanonical(t, true, rawdb.PathScheme)
}

func testExtendCanonical(t *testing.T, full bool, scheme string) {
	length := 5

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	better := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) <= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected more than %v", td2, td1)
		}
	}
	// Start fork from current height
	testFork(t, processor, length, 1, full, better, scheme)
	testFork(t, processor, length, 2, full, better, scheme)
	testFork(t, processor, length, 5, full, better, scheme)
	testFork(t, processor, length, 10, full, better, scheme)
}

// Tests that given a starting canonical chain of a given size, it can be extended
// with various length chains.
func TestExtendCanonicalHeadersAfterMerge(t *testing.T) {
	testExtendCanonicalAfterMerge(t, false, rawdb.HashScheme)
	testExtendCanonicalAfterMerge(t, false, rawdb.PathScheme)
}
func TestExtendCanonicalBlocksAfterMerge(t *testing.T) {
	testExtendCanonicalAfterMerge(t, true, rawdb.HashScheme)
	testExtendCanonicalAfterMerge(t, true, rawdb.PathScheme)
}

func testExtendCanonicalAfterMerge(t *testing.T, full bool, scheme string) {
	length := 5

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	testInsertAfterMerge(t, processor, length, 1, full, scheme)
	testInsertAfterMerge(t, processor, length, 10, full, scheme)
}

// Tests that given a starting canonical chain of a given size, creating shorter
// forks do not take canonical ownership.
func TestShorterForkHeaders(t *testing.T) {
	testShorterFork(t, false, rawdb.HashScheme)
	testShorterFork(t, false, rawdb.PathScheme)
}
func TestShorterForkBlocks(t *testing.T) {
	testShorterFork(t, true, rawdb.HashScheme)
	testShorterFork(t, true, rawdb.PathScheme)
}

func testShorterFork(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	worse := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) >= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected less than %v", td2, td1)
		}
	}
	// Sum of numbers must be less than `length` for this to be a shorter fork
	testFork(t, processor, 0, 3, full, worse, scheme)
	testFork(t, processor, 0, 7, full, worse, scheme)
	testFork(t, processor, 1, 1, full, worse, scheme)
	testFork(t, processor, 1, 7, full, worse, scheme)
	testFork(t, processor, 5, 3, full, worse, scheme)
	testFork(t, processor, 5, 4, full, worse, scheme)
}

// Tests that given a starting canonical chain of a given size, creating shorter
// forks do not take canonical ownership.
func TestShorterForkHeadersAfterMerge(t *testing.T) {
	testShorterForkAfterMerge(t, false, rawdb.HashScheme)
	testShorterForkAfterMerge(t, false, rawdb.PathScheme)
}
func TestShorterForkBlocksAfterMerge(t *testing.T) {
	testShorterForkAfterMerge(t, true, rawdb.HashScheme)
	testShorterForkAfterMerge(t, true, rawdb.PathScheme)
}

func testShorterForkAfterMerge(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	testInsertAfterMerge(t, processor, 0, 3, full, scheme)
	testInsertAfterMerge(t, processor, 0, 7, full, scheme)
	testInsertAfterMerge(t, processor, 1, 1, full, scheme)
	testInsertAfterMerge(t, processor, 1, 7, full, scheme)
	testInsertAfterMerge(t, processor, 5, 3, full, scheme)
	testInsertAfterMerge(t, processor, 5, 4, full, scheme)
}

// Tests that given a starting canonical chain of a given size, creating longer
// forks do take canonical ownership.
func TestLongerForkHeaders(t *testing.T) {
	testLongerFork(t, false, rawdb.HashScheme)
	testLongerFork(t, false, rawdb.PathScheme)
}
func TestLongerForkBlocks(t *testing.T) {
	testLongerFork(t, true, rawdb.HashScheme)
	testLongerFork(t, true, rawdb.PathScheme)
}

func testLongerFork(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	testInsertAfterMerge(t, processor, 0, 11, full, scheme)
	testInsertAfterMerge(t, processor, 0, 15, full, scheme)
	testInsertAfterMerge(t, processor, 1, 10, full, scheme)
	testInsertAfterMerge(t, processor, 1, 12, full, scheme)
	testInsertAfterMerge(t, processor, 5, 6, full, scheme)
	testInsertAfterMerge(t, processor, 5, 8, full, scheme)
}

// Tests that given a starting canonical chain of a given size, creating longer
// forks do take canonical ownership.
func TestLongerForkHeadersAfterMerge(t *testing.T) {
	testLongerForkAfterMerge(t, false, rawdb.HashScheme)
	testLongerForkAfterMerge(t, false, rawdb.PathScheme)
}
func TestLongerForkBlocksAfterMerge(t *testing.T) {
	testLongerForkAfterMerge(t, true, rawdb.HashScheme)
	testLongerForkAfterMerge(t, true, rawdb.PathScheme)
}

func testLongerForkAfterMerge(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	testInsertAfterMerge(t, processor, 0, 11, full, scheme)
	testInsertAfterMerge(t, processor, 0, 15, full, scheme)
	testInsertAfterMerge(t, processor, 1, 10, full, scheme)
	testInsertAfterMerge(t, processor, 1, 12, full, scheme)
	testInsertAfterMerge(t, processor, 5, 6, full, scheme)
	testInsertAfterMerge(t, processor, 5, 8, full, scheme)
}

// Tests that given a starting canonical chain of a given size, creating equal
// forks do take canonical ownership.
func TestEqualForkHeaders(t *testing.T) {
	testEqualFork(t, false, rawdb.HashScheme)
	testEqualFork(t, false, rawdb.PathScheme)
}
func TestEqualForkBlocks(t *testing.T) {
	testEqualFork(t, true, rawdb.HashScheme)
	testEqualFork(t, true, rawdb.PathScheme)
}

func testEqualFork(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	equal := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", td2, td1)
		}
	}
	// Sum of numbers must be equal to `length` for this to be an equal fork
	testFork(t, processor, 0, 10, full, equal, scheme)
	testFork(t, processor, 1, 9, full, equal, scheme)
	testFork(t, processor, 2, 8, full, equal, scheme)
	testFork(t, processor, 5, 5, full, equal, scheme)
	testFork(t, processor, 6, 4, full, equal, scheme)
	testFork(t, processor, 9, 1, full, equal, scheme)
}

// Tests that given a starting canonical chain of a given size, creating equal
// forks do take canonical ownership.
func TestEqualForkHeadersAfterMerge(t *testing.T) {
	testEqualForkAfterMerge(t, false, rawdb.HashScheme)
	testEqualForkAfterMerge(t, false, rawdb.PathScheme)
}
func TestEqualForkBlocksAfterMerge(t *testing.T) {
	testEqualForkAfterMerge(t, true, rawdb.HashScheme)
	testEqualForkAfterMerge(t, true, rawdb.PathScheme)
}

func testEqualForkAfterMerge(t *testing.T, full bool, scheme string) {
	length := 10

	// Make first chain starting from genesis
	_, _, processor, err := newCanonical(ethash.NewFaker(), length, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	testInsertAfterMerge(t, processor, 0, 10, full, scheme)
	testInsertAfterMerge(t, processor, 1, 9, full, scheme)
	testInsertAfterMerge(t, processor, 2, 8, full, scheme)
	testInsertAfterMerge(t, processor, 5, 5, full, scheme)
	testInsertAfterMerge(t, processor, 6, 4, full, scheme)
	testInsertAfterMerge(t, processor, 9, 1, full, scheme)
}

// Tests that chains missing links do not get accepted by the processor.
func TestBrokenHeaderChain(t *testing.T) {
	testBrokenChain(t, false, rawdb.HashScheme)
	testBrokenChain(t, false, rawdb.PathScheme)
}
func TestBrokenBlockChain(t *testing.T) {
	testBrokenChain(t, true, rawdb.HashScheme)
	testBrokenChain(t, true, rawdb.PathScheme)
}

func testBrokenChain(t *testing.T, full bool, scheme string) {
	// Make chain starting from genesis
	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 10, full, scheme)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer blockchain.Stop()

	// Create a forked chain, and try to insert with a missing link
	if full {
		chain := makeBlockChain(blockchain.chainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 5, ethash.NewFaker(), genDb, forkSeed)[1:]
		if err := testBlockChainImport(chain, blockchain); err == nil {
			t.Errorf("broken block chain not reported")
		}
	} else {
		chain := makeHeaderChain(blockchain.chainConfig, blockchain.CurrentHeader(), 5, ethash.NewFaker(), genDb, forkSeed)[1:]
		if err := testHeaderChainImport(chain, blockchain); err == nil {
			t.Errorf("broken header chain not reported")
		}
	}
}

// Tests that reorganising a long difficult chain after a short easy one
// overwrites the canonical numbers and links in the database.
func TestReorgLongHeaders(t *testing.T) {
	testReorgLong(t, false, rawdb.HashScheme)
	testReorgLong(t, false, rawdb.PathScheme)
}
func TestReorgLongBlocks(t *testing.T) {
	testReorgLong(t, true, rawdb.HashScheme)
	testReorgLong(t, true, rawdb.PathScheme)
}

func testReorgLong(t *testing.T, full bool, scheme string) {
	testReorg(t, []int64{0, 0, -9}, []int64{0, 0, 0, -9}, 393280+params.GenesisDifficulty.Int64(), full, scheme)
}

// Tests that reorganising a short difficult chain after a long easy one
// overwrites the canonical numbers and links in the database.
func TestReorgShortHeaders(t *testing.T) {
	testReorgShort(t, false, rawdb.HashScheme)
	testReorgShort(t, false, rawdb.PathScheme)
}
func TestReorgShortBlocks(t *testing.T) {
	testReorgShort(t, true, rawdb.HashScheme)
	testReorgShort(t, true, rawdb.PathScheme)
}

func testReorgShort(t *testing.T, full bool, scheme string) {
	// Create a long easy chain vs. a short heavy one. Due to difficulty adjustment
	// we need a fairly long chain of blocks with different difficulties for a short
	// one to become heavier than a long one. The 96 is an empirical value.
	easy := make([]int64, 96)
	for i := 0; i < len(easy); i++ {
		easy[i] = 60
	}
	diff := make([]int64, len(easy)-1)
	for i := 0; i < len(diff); i++ {
		diff[i] = -9
	}
	testReorg(t, easy, diff, 12615120+params.GenesisDifficulty.Int64(), full, scheme)
}

func testReorg(t *testing.T, first, second []int64, td int64, full bool, scheme string) {
	// Create a pristine chain and database
	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, full, scheme)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	// Insert an easy and a difficult chain afterwards
	easyBlocks, _ := GenerateChain(params.TestChainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), ethash.NewFaker(), genDb, len(first), func(i int, b *BlockGen) {
		b.OffsetTime(first[i])
	})
	diffBlocks, _ := GenerateChain(params.TestChainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), ethash.NewFaker(), genDb, len(second), func(i int, b *BlockGen) {
		b.OffsetTime(second[i])
	})
	if full {
		if _, err := blockchain.InsertChain(easyBlocks); err != nil {
			t.Fatalf("failed to insert easy chain: %v", err)
		}
		if _, err := blockchain.InsertChain(diffBlocks); err != nil {
			t.Fatalf("failed to insert difficult chain: %v", err)
		}
	} else {
		easyHeaders := make([]*types.Header, len(easyBlocks))
		for i, block := range easyBlocks {
			easyHeaders[i] = block.Header()
		}
		diffHeaders := make([]*types.Header, len(diffBlocks))
		for i, block := range diffBlocks {
			diffHeaders[i] = block.Header()
		}
		if _, err := blockchain.InsertHeaderChain(easyHeaders); err != nil {
			t.Fatalf("failed to insert easy chain: %v", err)
		}
		if _, err := blockchain.InsertHeaderChain(diffHeaders); err != nil {
			t.Fatalf("failed to insert difficult chain: %v", err)
		}
	}
	// Check that the chain is valid number and link wise
	if full {
		prev := blockchain.CurrentBlock()
		for block := blockchain.GetBlockByNumber(blockchain.CurrentBlock().Number.Uint64() - 1); block.NumberU64() != 0; prev, block = block.Header(), blockchain.GetBlockByNumber(block.NumberU64()-1) {
			if prev.ParentHash != block.Hash() {
				t.Errorf("parent block hash mismatch: have %x, want %x", prev.ParentHash, block.Hash())
			}
		}
	} else {
		prev := blockchain.CurrentHeader()
		for header := blockchain.GetHeaderByNumber(blockchain.CurrentHeader().Number.Uint64() - 1); header.Number.Uint64() != 0; prev, header = header, blockchain.GetHeaderByNumber(header.Number.Uint64()-1) {
			if prev.ParentHash != header.Hash() {
				t.Errorf("parent header hash mismatch: have %x, want %x", prev.ParentHash, header.Hash())
			}
		}
	}
	// Make sure the chain total difficulty is the correct one
	want := new(big.Int).Add(blockchain.genesisBlock.Difficulty(), big.NewInt(td))
	if full {
		cur := blockchain.CurrentBlock()
		if have := blockchain.GetTd(cur.Hash(), cur.Number.Uint64()); have.Cmp(want) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", have, want)
		}
	} else {
		cur := blockchain.CurrentHeader()
		if have := blockchain.GetTd(cur.Hash(), cur.Number.Uint64()); have.Cmp(want) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", have, want)
		}
	}
}

// Tests that the insertion functions detect banned hashes.
func TestBadHeaderHashes(t *testing.T) {
	testBadHashes(t, false, rawdb.HashScheme)
	testBadHashes(t, false, rawdb.PathScheme)
}

func TestBadBlockHashes(t *testing.T) {
	testBadHashes(t, true, rawdb.HashScheme)
	testBadHashes(t, true, rawdb.PathScheme)
}

func testBadHashes(t *testing.T, full bool, scheme string) {
	// Create a pristine chain and database
	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, full, scheme)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	// Create a chain, ban a hash and try to import
	if full {
		blocks := makeBlockChain(blockchain.chainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 3, ethash.NewFaker(), genDb, 10)

		BadHashes[blocks[2].Header().Hash()] = true
		defer func() { delete(BadHashes, blocks[2].Header().Hash()) }()

		_, err = blockchain.InsertChain(blocks)
	} else {
		headers := makeHeaderChain(blockchain.chainConfig, blockchain.CurrentHeader(), 3, ethash.NewFaker(), genDb, 10)

		BadHashes[headers[2].Hash()] = true
		defer func() { delete(BadHashes, headers[2].Hash()) }()

		_, err = blockchain.InsertHeaderChain(headers)
	}
	if !errors.Is(err, ErrBannedHash) {
		t.Errorf("error mismatch: have: %v, want: %v", err, ErrBannedHash)
	}
}

// Tests that bad hashes are detected on boot, and the chain rolled back to a
// good state prior to the bad hash.
func TestReorgBadHeaderHashes(t *testing.T) {
	testReorgBadHashes(t, false, rawdb.HashScheme)
	testReorgBadHashes(t, false, rawdb.PathScheme)
}
func TestReorgBadBlockHashes(t *testing.T) {
	testReorgBadHashes(t, true, rawdb.HashScheme)
	testReorgBadHashes(t, true, rawdb.PathScheme)
}

func testReorgBadHashes(t *testing.T, full bool, scheme string) {
	// Create a pristine chain and database
	genDb, gspec, blockchain, err := newCanonical(ethash.NewFaker(), 0, full, scheme)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	// Create a chain, import and ban afterwards
	headers := makeHeaderChain(blockchain.chainConfig, blockchain.CurrentHeader(), 4, ethash.NewFaker(), genDb, 10)
	blocks := makeBlockChain(blockchain.chainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 4, ethash.NewFaker(), genDb, 10)

	if full {
		if _, err = blockchain.InsertChain(blocks); err != nil {
			t.Errorf("failed to import blocks: %v", err)
		}
		if blockchain.CurrentBlock().Hash() != blocks[3].Hash() {
			t.Errorf("last block hash mismatch: have: %x, want %x", blockchain.CurrentBlock().Hash(), blocks[3].Header().Hash())
		}
		BadHashes[blocks[3].Header().Hash()] = true
		defer func() { delete(BadHashes, blocks[3].Header().Hash()) }()
	} else {
		if _, err = blockchain.InsertHeaderChain(headers); err != nil {
			t.Errorf("failed to import headers: %v", err)
		}
		if blockchain.CurrentHeader().Hash() != headers[3].Hash() {
			t.Errorf("last header hash mismatch: have: %x, want %x", blockchain.CurrentHeader().Hash(), headers[3].Hash())
		}
		BadHashes[headers[3].Hash()] = true
		defer func() { delete(BadHashes, headers[3].Hash()) }()
	}
	blockchain.Stop()

	// Create a new BlockChain and check that it rolled back the state.
	ncm, err := NewBlockChain(blockchain.db, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new chain manager: %v", err)
	}
	if full {
		if ncm.CurrentBlock().Hash() != blocks[2].Header().Hash() {
			t.Errorf("last block hash mismatch: have: %x, want %x", ncm.CurrentBlock().Hash(), blocks[2].Header().Hash())
		}
		if blocks[2].Header().GasLimit != ncm.GasLimit() {
			t.Errorf("last  block gasLimit mismatch: have: %d, want %d", ncm.GasLimit(), blocks[2].Header().GasLimit)
		}
	} else {
		if ncm.CurrentHeader().Hash() != headers[2].Hash() {
			t.Errorf("last header hash mismatch: have: %x, want %x", ncm.CurrentHeader().Hash(), headers[2].Hash())
		}
	}
	ncm.Stop()
}

// Tests chain insertions in the face of one entity containing an invalid nonce.
func TestHeadersInsertNonceError(t *testing.T) {
	testInsertNonceError(t, false, rawdb.HashScheme)
	testInsertNonceError(t, false, rawdb.PathScheme)
}
func TestBlocksInsertNonceError(t *testing.T) {
	testInsertNonceError(t, true, rawdb.HashScheme)
	testInsertNonceError(t, true, rawdb.PathScheme)
}

func testInsertNonceError(t *testing.T, full bool, scheme string) {
	doTest := func(i int) {
		// Create a pristine chain and database
		genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, full, scheme)
		if err != nil {
			t.Fatalf("failed to create pristine chain: %v", err)
		}
		defer blockchain.Stop()

		// Create and insert a chain with a failing nonce
		var (
			failAt  int
			failRes int
			failNum uint64
		)
		if full {
			blocks := makeBlockChain(blockchain.chainConfig, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), i, ethash.NewFaker(), genDb, 0)

			failAt = rand.Int() % len(blocks)
			failNum = blocks[failAt].NumberU64()

			blockchain.engine = ethash.NewFakeFailer(failNum)
			failRes, err = blockchain.InsertChain(blocks)
		} else {
			headers := makeHeaderChain(blockchain.chainConfig, blockchain.CurrentHeader(), i, ethash.NewFaker(), genDb, 0)

			failAt = rand.Int() % len(headers)
			failNum = headers[failAt].Number.Uint64()

			blockchain.engine = ethash.NewFakeFailer(failNum)
			blockchain.hc.engine = blockchain.engine
			failRes, err = blockchain.InsertHeaderChain(headers)
		}
		// Check that the returned error indicates the failure
		if failRes != failAt {
			t.Errorf("test %d: failure (%v) index mismatch: have %d, want %d", i, err, failRes, failAt)
		}
		// Check that all blocks after the failing block have been inserted
		for j := 0; j < i-failAt; j++ {
			if full {
				if block := blockchain.GetBlockByNumber(failNum + uint64(j)); block != nil {
					t.Errorf("test %d: invalid block in chain: %v", i, block)
				}
			} else {
				if header := blockchain.GetHeaderByNumber(failNum + uint64(j)); header != nil {
					t.Errorf("test %d: invalid header in chain: %v", i, header)
				}
			}
		}
	}
	for i := 1; i < 25 && !t.Failed(); i++ {
		doTest(i)
	}
}

// Tests that fast importing a block chain produces the same chain data as the
// classical full block processing.
func TestFastVsFullChains(t *testing.T) {
	testFastVsFullChains(t, rawdb.HashScheme)
	testFastVsFullChains(t, rawdb.PathScheme)
}

func testFastVsFullChains(t *testing.T, scheme string) {
	// Configure and generate a sample block chain
	var (
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		gspec   = &Genesis{
			Config:  params.TestChainConfig,
			Alloc:   types.GenesisAlloc{address: {Balance: funds}},
			BaseFee: big.NewInt(params.InitialBaseFee),
		}
		signer = types.LatestSigner(gspec.Config)
	)
	_, blocks, receipts := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 1024, func(i int, block *BlockGen) {
		block.SetCoinbase(common.Address{0x00})

		// If the block number is multiple of 3, send a few bonus transactions to the miner
		if i%3 == 2 {
			for j := 0; j < i%4+1; j++ {
				tx, err := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{0x00}, big.NewInt(1000), params.TxGas, block.header.BaseFee, nil), signer, key)
				if err != nil {
					panic(err)
				}
				block.AddTx(tx)
			}
		}
		// If the block number is a multiple of 5, add an uncle to the block
		if i%5 == 4 {
			block.AddUncle(&types.Header{ParentHash: block.PrevBlock(i - 2).Hash(), Number: big.NewInt(int64(i))})
		}
	})
	// Import the chain as an archive node for the comparison baseline
	archiveDb := rawdb.NewMemoryDatabase()
	archive, _ := NewBlockChain(archiveDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer archive.Stop()

	if n, err := archive.InsertChain(blocks); err != nil {
		t.Fatalf("failed to process block %d: %v", n, err)
	}
	// Fast import the chain as a non-archive node to test
	fastDb := rawdb.NewMemoryDatabase()
	fast, _ := NewBlockChain(fastDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer fast.Stop()

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := fast.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := fast.InsertReceiptChain(blocks, receipts, 0); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	// Freezer style fast import the chain.
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	defer ancientDb.Close()

	ancient, _ := NewBlockChain(ancientDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()

	if n, err := ancient.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(len(blocks)/2)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}

	// Iterate over all chain data components, and cross reference
	for i := 0; i < len(blocks); i++ {
		num, hash, time := blocks[i].NumberU64(), blocks[i].Hash(), blocks[i].Time()

		if ftd, atd := fast.GetTd(hash, num), archive.GetTd(hash, num); ftd.Cmp(atd) != 0 {
			t.Errorf("block #%d [%x]: td mismatch: fastdb %v, archivedb %v", num, hash, ftd, atd)
		}
		if antd, artd := ancient.GetTd(hash, num), archive.GetTd(hash, num); antd.Cmp(artd) != 0 {
			t.Errorf("block #%d [%x]: td mismatch: ancientdb %v, archivedb %v", num, hash, antd, artd)
		}
		if fheader, aheader := fast.GetHeaderByHash(hash), archive.GetHeaderByHash(hash); fheader.Hash() != aheader.Hash() {
			t.Errorf("block #%d [%x]: header mismatch: fastdb %v, archivedb %v", num, hash, fheader, aheader)
		}
		if anheader, arheader := ancient.GetHeaderByHash(hash), archive.GetHeaderByHash(hash); anheader.Hash() != arheader.Hash() {
			t.Errorf("block #%d [%x]: header mismatch: ancientdb %v, archivedb %v", num, hash, anheader, arheader)
		}
		if fblock, arblock, anblock := fast.GetBlockByHash(hash), archive.GetBlockByHash(hash), ancient.GetBlockByHash(hash); fblock.Hash() != arblock.Hash() || anblock.Hash() != arblock.Hash() {
			t.Errorf("block #%d [%x]: block mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, fblock, anblock, arblock)
		} else if types.DeriveSha(fblock.Transactions(), trie.NewStackTrie(nil)) != types.DeriveSha(arblock.Transactions(), trie.NewStackTrie(nil)) || types.DeriveSha(anblock.Transactions(), trie.NewStackTrie(nil)) != types.DeriveSha(arblock.Transactions(), trie.NewStackTrie(nil)) {
			t.Errorf("block #%d [%x]: transactions mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, fblock.Transactions(), anblock.Transactions(), arblock.Transactions())
		} else if types.CalcUncleHash(fblock.Uncles()) != types.CalcUncleHash(arblock.Uncles()) || types.CalcUncleHash(anblock.Uncles()) != types.CalcUncleHash(arblock.Uncles()) {
			t.Errorf("block #%d [%x]: uncles mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, fblock.Uncles(), anblock, arblock.Uncles())
		}

		// Check receipts.
		freceipts := rawdb.ReadReceipts(fastDb, hash, num, time, fast.Config())
		anreceipts := rawdb.ReadReceipts(ancientDb, hash, num, time, fast.Config())
		areceipts := rawdb.ReadReceipts(archiveDb, hash, num, time, fast.Config())
		if types.DeriveSha(freceipts, trie.NewStackTrie(nil)) != types.DeriveSha(areceipts, trie.NewStackTrie(nil)) {
			t.Errorf("block #%d [%x]: receipts mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, freceipts, anreceipts, areceipts)
		}

		// Check that hash-to-number mappings are present in all databases.
		if m := rawdb.ReadHeaderNumber(fastDb, hash); m == nil || *m != num {
			t.Errorf("block #%d [%x]: wrong hash-to-number mapping in fastdb: %v", num, hash, m)
		}
		if m := rawdb.ReadHeaderNumber(ancientDb, hash); m == nil || *m != num {
			t.Errorf("block #%d [%x]: wrong hash-to-number mapping in ancientdb: %v", num, hash, m)
		}
		if m := rawdb.ReadHeaderNumber(archiveDb, hash); m == nil || *m != num {
			t.Errorf("block #%d [%x]: wrong hash-to-number mapping in archivedb: %v", num, hash, m)
		}
	}

	// Check that the canonical chains are the same between the databases
	for i := 0; i < len(blocks)+1; i++ {
		if fhash, ahash := rawdb.ReadCanonicalHash(fastDb, uint64(i)), rawdb.ReadCanonicalHash(archiveDb, uint64(i)); fhash != ahash {
			t.Errorf("block #%d: canonical hash mismatch: fastdb %v, archivedb %v", i, fhash, ahash)
		}
		if anhash, arhash := rawdb.ReadCanonicalHash(ancientDb, uint64(i)), rawdb.ReadCanonicalHash(archiveDb, uint64(i)); anhash != arhash {
			t.Errorf("block #%d: canonical hash mismatch: ancientdb %v, archivedb %v", i, anhash, arhash)
		}
	}
}

// Tests that various import methods move the chain head pointers to the correct
// positions.
func TestLightVsFastVsFullChainHeads(t *testing.T) {
	testLightVsFastVsFullChainHeads(t, rawdb.HashScheme)
	testLightVsFastVsFullChainHeads(t, rawdb.PathScheme)
}

func testLightVsFastVsFullChainHeads(t *testing.T, scheme string) {
	// Configure and generate a sample block chain
	var (
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		gspec   = &Genesis{
			Config:  params.TestChainConfig,
			Alloc:   types.GenesisAlloc{address: {Balance: funds}},
			BaseFee: big.NewInt(params.InitialBaseFee),
		}
	)
	height := uint64(1024)
	_, blocks, receipts := GenerateChainWithGenesis(gspec, ethash.NewFaker(), int(height), nil)

	// makeDb creates a db instance for testing.
	makeDb := func() ethdb.Database {
		db, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
		if err != nil {
			t.Fatalf("failed to create temp freezer db: %v", err)
		}
		return db
	}
	// Configure a subchain to roll back
	remove := blocks[height/2].NumberU64()

	// Create a small assertion method to check the three heads
	assert := func(t *testing.T, kind string, chain *BlockChain, header uint64, fast uint64, block uint64) {
		t.Helper()

		if num := chain.CurrentBlock().Number.Uint64(); num != block {
			t.Errorf("%s head block mismatch: have #%v, want #%v", kind, num, block)
		}
		if num := chain.CurrentSnapBlock().Number.Uint64(); num != fast {
			t.Errorf("%s head snap-block mismatch: have #%v, want #%v", kind, num, fast)
		}
		if num := chain.CurrentHeader().Number.Uint64(); num != header {
			t.Errorf("%s head header mismatch: have #%v, want #%v", kind, num, header)
		}
	}
	// Import the chain as an archive node and ensure all pointers are updated
	archiveDb := makeDb()
	defer archiveDb.Close()

	archiveCaching := *defaultCacheConfig
	archiveCaching.TrieDirtyDisabled = true
	archiveCaching.StateScheme = scheme

	archive, _ := NewBlockChain(archiveDb, &archiveCaching, gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	if n, err := archive.InsertChain(blocks); err != nil {
		t.Fatalf("failed to process block %d: %v", n, err)
	}
	defer archive.Stop()

	assert(t, "archive", archive, height, height, height)
	archive.SetHead(remove - 1)
	assert(t, "archive", archive, height/2, height/2, height/2)

	// Import the chain as a non-archive node and ensure all pointers are updated
	fastDb := makeDb()
	defer fastDb.Close()
	fast, _ := NewBlockChain(fastDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer fast.Stop()

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := fast.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := fast.InsertReceiptChain(blocks, receipts, 0); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	assert(t, "fast", fast, height, height, 0)
	fast.SetHead(remove - 1)
	assert(t, "fast", fast, height/2, height/2, 0)

	// Import the chain as a ancient-first node and ensure all pointers are updated
	ancientDb := makeDb()
	defer ancientDb.Close()
	ancient, _ := NewBlockChain(ancientDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()

	if n, err := ancient.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	assert(t, "ancient", ancient, height, height, 0)
	ancient.SetHead(remove - 1)
	assert(t, "ancient", ancient, 0, 0, 0)

	if frozen, err := ancientDb.Ancients(); err != nil || frozen != 1 {
		t.Fatalf("failed to truncate ancient store, want %v, have %v", 1, frozen)
	}
	// Import the chain as a light node and ensure all pointers are updated
	lightDb := makeDb()
	defer lightDb.Close()
	light, _ := NewBlockChain(lightDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	if n, err := light.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	defer light.Stop()

	assert(t, "light", light, height, 0, 0)
	light.SetHead(remove - 1)
	assert(t, "light", light, height/2, 0, 0)
}

// Tests that chain reorganisations handle transaction removals and reinsertions.
func TestChainTxReorgs(t *testing.T) {
	testChainTxReorgs(t, rawdb.HashScheme)
	testChainTxReorgs(t, rawdb.PathScheme)
}

func testChainTxReorgs(t *testing.T, scheme string) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		gspec   = &Genesis{
			Config:   params.TestChainConfig,
			GasLimit: 3141592,
			Alloc: types.GenesisAlloc{
				addr1: {Balance: big.NewInt(1000000000000000)},
				addr2: {Balance: big.NewInt(1000000000000000)},
				addr3: {Balance: big.NewInt(1000000000000000)},
			},
		}
		signer = types.LatestSigner(gspec.Config)
	)

	// Create two transactions shared between the chains:
	//  - postponed: transaction included at a later block in the forked chain
	//  - swapped: transaction included at the same block number in the forked chain
	postponed, _ := types.SignTx(types.NewTransaction(0, addr1, big.NewInt(1000), params.TxGas, big.NewInt(params.InitialBaseFee), nil), signer, key1)
	swapped, _ := types.SignTx(types.NewTransaction(1, addr1, big.NewInt(1000), params.TxGas, big.NewInt(params.InitialBaseFee), nil), signer, key1)

	// Create two transactions that will be dropped by the forked chain:
	//  - pastDrop: transaction dropped retroactively from a past block
	//  - freshDrop: transaction dropped exactly at the block where the reorg is detected
	var pastDrop, freshDrop *types.Transaction

	// Create three transactions that will be added in the forked chain:
	//  - pastAdd:   transaction added before the reorganization is detected
	//  - freshAdd:  transaction added at the exact block the reorg is detected
	//  - futureAdd: transaction added after the reorg has already finished
	var pastAdd, freshAdd, futureAdd *types.Transaction

	_, chain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 3, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			pastDrop, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr2, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key2)

			gen.AddTx(pastDrop)  // This transaction will be dropped in the fork from below the split point
			gen.AddTx(postponed) // This transaction will be postponed till block #3 in the fork

		case 2:
			freshDrop, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr2, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key2)

			gen.AddTx(freshDrop) // This transaction will be dropped in the fork from exactly at the split point
			gen.AddTx(swapped)   // This transaction will be swapped out at the exact height

			gen.OffsetTime(9) // Lower the block difficulty to simulate a weaker chain
		}
	})
	// Import the chain. This runs all block validation rules.
	db := rawdb.NewMemoryDatabase()
	blockchain, _ := NewBlockChain(db, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	if i, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert original chain[%d]: %v", i, err)
	}
	defer blockchain.Stop()

	// overwrite the old chain
	_, chain, _ = GenerateChainWithGenesis(gspec, ethash.NewFaker(), 5, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			pastAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key3)
			gen.AddTx(pastAdd) // This transaction needs to be injected during reorg

		case 2:
			gen.AddTx(postponed) // This transaction was postponed from block #1 in the original chain
			gen.AddTx(swapped)   // This transaction was swapped from the exact current spot in the original chain

			freshAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key3)
			gen.AddTx(freshAdd) // This transaction will be added exactly at reorg time

		case 3:
			futureAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key3)
			gen.AddTx(futureAdd) // This transaction will be added after a full reorg
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}

	// removed tx
	for i, tx := range (types.Transactions{pastDrop, freshDrop}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn != nil {
			t.Errorf("drop %d: tx %v found while shouldn't have been", i, txn)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt != nil {
			t.Errorf("drop %d: receipt %v found while shouldn't have been", i, rcpt)
		}
	}
	// added tx
	for i, tx := range (types.Transactions{pastAdd, freshAdd, futureAdd}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn == nil {
			t.Errorf("add %d: expected tx to be found", i)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt == nil {
			t.Errorf("add %d: expected receipt to be found", i)
		}
	}
	// shared tx
	for i, tx := range (types.Transactions{postponed, swapped}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn == nil {
			t.Errorf("share %d: expected tx to be found", i)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt == nil {
			t.Errorf("share %d: expected receipt to be found", i)
		}
	}
}

func TestLogReorgs(t *testing.T) {
	testLogReorgs(t, rawdb.HashScheme)
	testLogReorgs(t, rawdb.PathScheme)
}

func testLogReorgs(t *testing.T, scheme string) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)

		// this code generates a log
		code   = common.Hex2Bytes("60606040525b7f24ec1d3ff24c2f6ff210738839dbc339cd45a5294d85c79361016243157aae7b60405180905060405180910390a15b600a8060416000396000f360606040526008565b00")
		gspec  = &Genesis{Config: params.TestChainConfig, Alloc: types.GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000000)}}}
		signer = types.LatestSigner(gspec.Config)
	)

	blockchain, _ := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	rmLogsCh := make(chan RemovedLogsEvent)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)
	_, chain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 2, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, gen.header.BaseFee, code), signer, key1)
			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	_, chain, _ = GenerateChainWithGenesis(gspec, ethash.NewFaker(), 3, func(i int, gen *BlockGen) {})
	done := make(chan struct{})
	go func() {
		ev := <-rmLogsCh
		if len(ev.Logs) == 0 {
			t.Error("expected logs")
		}
		close(done)
	}()
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	timeout := time.NewTimer(1 * time.Second)
	defer timeout.Stop()
	select {
	case <-done:
	case <-timeout.C:
		t.Fatal("Timeout. There is no RemovedLogsEvent has been sent.")
	}
}

// This EVM code generates a log when the contract is created.
var logCode = common.Hex2Bytes("60606040525b7f24ec1d3ff24c2f6ff210738839dbc339cd45a5294d85c79361016243157aae7b60405180905060405180910390a15b600a8060416000396000f360606040526008565b00")

// This test checks that log events and RemovedLogsEvent are sent
// when the chain reorganizes.
func TestLogRebirth(t *testing.T) {
	testLogRebirth(t, rawdb.HashScheme)
	testLogRebirth(t, rawdb.PathScheme)
}

func testLogRebirth(t *testing.T, scheme string) {
	var (
		key1, _       = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1         = crypto.PubkeyToAddress(key1.PublicKey)
		gspec         = &Genesis{Config: params.TestChainConfig, Alloc: types.GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000000)}}}
		signer        = types.LatestSigner(gspec.Config)
		engine        = ethash.NewFaker()
		blockchain, _ = NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
	)
	defer blockchain.Stop()

	// The event channels.
	newLogCh := make(chan []*types.Log, 10)
	rmLogsCh := make(chan RemovedLogsEvent, 10)
	blockchain.SubscribeLogsEvent(newLogCh)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)

	// This chain contains 10 logs.
	genDb, chain, _ := GenerateChainWithGenesis(gspec, engine, 3, func(i int, gen *BlockGen) {
		if i < 2 {
			for ii := 0; ii < 5; ii++ {
				tx, err := types.SignNewTx(key1, signer, &types.LegacyTx{
					Nonce:    gen.TxNonce(addr1),
					GasPrice: gen.header.BaseFee,
					Gas:      uint64(1000001),
					Data:     logCode,
				})
				if err != nil {
					t.Fatalf("failed to create tx: %v", err)
				}
				gen.AddTx(tx)
			}
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 10, 0)

	// Generate long reorg chain containing more logs. Inserting the
	// chain removes one log and adds four.
	_, forkChain, _ := GenerateChainWithGenesis(gspec, engine, 3, func(i int, gen *BlockGen) {
		if i == 2 {
			// The last (head) block is not part of the reorg-chain, we can ignore it
			return
		}
		for ii := 0; ii < 5; ii++ {
			tx, err := types.SignNewTx(key1, signer, &types.LegacyTx{
				Nonce:    gen.TxNonce(addr1),
				GasPrice: gen.header.BaseFee,
				Gas:      uint64(1000000),
				Data:     logCode,
			})
			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
		gen.OffsetTime(-9) // higher block difficulty
	})
	if _, err := blockchain.InsertChain(forkChain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 10, 10)

	// This chain segment is rooted in the original chain, but doesn't contain any logs.
	// When inserting it, the canonical chain switches away from forkChain and re-emits
	// the log event for the old chain, as well as a RemovedLogsEvent for forkChain.
	newBlocks, _ := GenerateChain(gspec.Config, chain[len(chain)-1], engine, genDb, 1, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(newBlocks); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 10, 10)
}

// This test is a variation of TestLogRebirth. It verifies that log events are emitted
// when a side chain containing log events overtakes the canonical chain.
func TestSideLogRebirth(t *testing.T) {
	testSideLogRebirth(t, rawdb.HashScheme)
	testSideLogRebirth(t, rawdb.PathScheme)
}

func testSideLogRebirth(t *testing.T, scheme string) {
	var (
		key1, _       = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1         = crypto.PubkeyToAddress(key1.PublicKey)
		gspec         = &Genesis{Config: params.TestChainConfig, Alloc: types.GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000000)}}}
		signer        = types.LatestSigner(gspec.Config)
		blockchain, _ = NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	)
	defer blockchain.Stop()

	newLogCh := make(chan []*types.Log, 10)
	rmLogsCh := make(chan RemovedLogsEvent, 10)
	blockchain.SubscribeLogsEvent(newLogCh)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)

	_, chain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 2, func(i int, gen *BlockGen) {
		if i == 1 {
			gen.OffsetTime(-9) // higher block difficulty
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 0, 0)

	// Generate side chain with lower difficulty
	genDb, sideChain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 2, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, gen.header.BaseFee, logCode), signer, key1)
			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
	})
	if _, err := blockchain.InsertChain(sideChain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 0, 0)

	// Generate a new block based on side chain.
	newBlocks, _ := GenerateChain(gspec.Config, sideChain[len(sideChain)-1], ethash.NewFaker(), genDb, 1, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(newBlocks); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 1, 0)
}

func checkLogEvents(t *testing.T, logsCh <-chan []*types.Log, rmLogsCh <-chan RemovedLogsEvent, wantNew, wantRemoved int) {
	t.Helper()
	var (
		countNew int
		countRm  int
		prev     int
	)
	// Drain events.
	for len(logsCh) > 0 {
		x := <-logsCh
		countNew += len(x)
		for _, log := range x {
			// We expect added logs to be in ascending order: 0:0, 0:1, 1:0 ...
			have := 100*int(log.BlockNumber) + int(log.TxIndex)
			if have < prev {
				t.Fatalf("Expected new logs to arrive in ascending order (%d < %d)", have, prev)
			}
			prev = have
		}
	}
	prev = 0
	for len(rmLogsCh) > 0 {
		x := <-rmLogsCh
		countRm += len(x.Logs)
		for _, log := range x.Logs {
			// We expect removed logs to be in ascending order: 0:0, 0:1, 1:0 ...
			have := 100*int(log.BlockNumber) + int(log.TxIndex)
			if have < prev {
				t.Fatalf("Expected removed logs to arrive in ascending order (%d < %d)", have, prev)
			}
			prev = have
		}
	}

	if countNew != wantNew {
		t.Fatalf("wrong number of log events: got %d, want %d", countNew, wantNew)
	}
	if countRm != wantRemoved {
		t.Fatalf("wrong number of removed log events: got %d, want %d", countRm, wantRemoved)
	}
}

func TestReorgSideEvent(t *testing.T) {
	testReorgSideEvent(t, rawdb.HashScheme)
	testReorgSideEvent(t, rawdb.PathScheme)
}

func testReorgSideEvent(t *testing.T, scheme string) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		gspec   = &Genesis{
			Config: params.TestChainConfig,
			Alloc:  types.GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000000)}},
		}
		signer = types.LatestSigner(gspec.Config)
	)
	blockchain, _ := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	_, chain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 3, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	_, replacementBlocks, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 4, func(i int, gen *BlockGen) {
		tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, gen.header.BaseFee, nil), signer, key1)
		if i == 2 {
			gen.OffsetTime(-9)
		}
		if err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}
		gen.AddTx(tx)
	})
	chainSideCh := make(chan ChainSideEvent, 64)
	blockchain.SubscribeChainSideEvent(chainSideCh)
	if _, err := blockchain.InsertChain(replacementBlocks); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	// first two block of the secondary chain are for a brief moment considered
	// side chains because up to that point the first one is considered the
	// heavier chain.
	expectedSideHashes := map[common.Hash]bool{
		replacementBlocks[0].Hash(): true,
		replacementBlocks[1].Hash(): true,
		chain[0].Hash():             true,
		chain[1].Hash():             true,
		chain[2].Hash():             true,
	}

	i := 0

	const timeoutDura = 10 * time.Second
	timeout := time.NewTimer(timeoutDura)
done:
	for {
		select {
		case ev := <-chainSideCh:
			block := ev.Block
			if _, ok := expectedSideHashes[block.Hash()]; !ok {
				t.Errorf("%d: didn't expect %x to be in side chain", i, block.Hash())
			}
			i++

			if i == len(expectedSideHashes) {
				timeout.Stop()

				break done
			}
			timeout.Reset(timeoutDura)

		case <-timeout.C:
			t.Fatal("Timeout. Possibly not all blocks were triggered for sideevent")
		}
	}

	// make sure no more events are fired
	select {
	case e := <-chainSideCh:
		t.Errorf("unexpected event fired: %v", e)
	case <-time.After(250 * time.Millisecond):
	}
}

// Tests if the canonical block can be fetched from the database during chain insertion.
func TestCanonicalBlockRetrieval(t *testing.T) {
	testCanonicalBlockRetrieval(t, rawdb.HashScheme)
	testCanonicalBlockRetrieval(t, rawdb.PathScheme)
}

func testCanonicalBlockRetrieval(t *testing.T, scheme string) {
	_, gspec, blockchain, err := newCanonical(ethash.NewFaker(), 0, true, scheme)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	_, chain, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 10, func(i int, gen *BlockGen) {})

	var pend sync.WaitGroup
	pend.Add(len(chain))

	for i := range chain {
		go func(block *types.Block) {
			defer pend.Done()

			// try to retrieve a block by its canonical hash and see if the block data can be retrieved.
			for {
				ch := rawdb.ReadCanonicalHash(blockchain.db, block.NumberU64())
				if ch == (common.Hash{}) {
					continue // busy wait for canonical hash to be written
				}
				if ch != block.Hash() {
					t.Errorf("unknown canonical hash, want %s, got %s", block.Hash().Hex(), ch.Hex())
					return
				}
				fb := rawdb.ReadBlock(blockchain.db, ch, block.NumberU64())
				if fb == nil {
					t.Errorf("unable to retrieve block %d for canonical hash: %s", block.NumberU64(), ch.Hex())
					return
				}
				if fb.Hash() != block.Hash() {
					t.Errorf("invalid block hash for block %d, want %s, got %s", block.NumberU64(), block.Hash().Hex(), fb.Hash().Hex())
					return
				}
				return
			}
		}(chain[i])

		if _, err := blockchain.InsertChain(types.Blocks{chain[i]}); err != nil {
			t.Fatalf("failed to insert block %d: %v", i, err)
		}
	}
	pend.Wait()
}
func TestEIP155Transition(t *testing.T) {
	testEIP155Transition(t, rawdb.HashScheme)
	testEIP155Transition(t, rawdb.PathScheme)
}

func testEIP155Transition(t *testing.T, scheme string) {
	// Configure and generate a sample block chain
	var (
		key, _     = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address    = crypto.PubkeyToAddress(key.PublicKey)
		funds      = big.NewInt(1000000000)
		deleteAddr = common.Address{1}
		gspec      = &Genesis{
			Config: &params.ChainConfig{
				ChainID:        big.NewInt(1),
				EIP150Block:    big.NewInt(0),
				EIP155Block:    big.NewInt(2),
				HomesteadBlock: new(big.Int),
			},
			Alloc: types.GenesisAlloc{address: {Balance: funds}, deleteAddr: {Balance: new(big.Int)}},
		}
	)
	genDb, blocks, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 4, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				return types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), 21000, new(big.Int), nil), signer, key)
			}
		)
		switch i {
		case 0:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 2:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.LatestSigner(gspec.Config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 3:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.LatestSigner(gspec.Config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})

	blockchain, _ := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	block := blockchain.GetBlockByNumber(1)
	if block.Transactions()[0].Protected() {
		t.Error("Expected block[0].txs[0] to not be replay protected")
	}

	block = blockchain.GetBlockByNumber(3)
	if block.Transactions()[0].Protected() {
		t.Error("Expected block[3].txs[0] to not be replay protected")
	}
	if !block.Transactions()[1].Protected() {
		t.Error("Expected block[3].txs[1] to be replay protected")
	}
	if _, err := blockchain.InsertChain(blocks[4:]); err != nil {
		t.Fatal(err)
	}

	// generate an invalid chain id transaction
	config := &params.ChainConfig{
		ChainID:        big.NewInt(2),
		EIP150Block:    big.NewInt(0),
		EIP155Block:    big.NewInt(2),
		HomesteadBlock: new(big.Int),
	}
	blocks, _ = GenerateChain(config, blocks[len(blocks)-1], ethash.NewFaker(), genDb, 4, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				return types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), 21000, new(big.Int), nil), signer, key)
			}
		)
		if i == 0 {
			tx, err = basicTx(types.LatestSigner(config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})
	_, err := blockchain.InsertChain(blocks)
	if have, want := err, types.ErrInvalidChainId; !errors.Is(have, want) {
		t.Errorf("have %v, want %v", have, want)
	}
}
func TestEIP161AccountRemoval(t *testing.T) {
	testEIP161AccountRemoval(t, rawdb.HashScheme)
	testEIP161AccountRemoval(t, rawdb.PathScheme)
}

func testEIP161AccountRemoval(t *testing.T, scheme string) {
	// Configure and generate a sample block chain
	var (
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		theAddr = common.Address{1}
		gspec   = &Genesis{
			Config: &params.ChainConfig{
				ChainID:        big.NewInt(1),
				HomesteadBlock: new(big.Int),
				EIP155Block:    new(big.Int),
				EIP150Block:    new(big.Int),
				EIP158Block:    big.NewInt(2),
			},
			Alloc: types.GenesisAlloc{address: {Balance: funds}},
		}
	)
	_, blocks, _ := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 3, func(i int, block *BlockGen) {
		var (
			tx     *types.Transaction
			err    error
			signer = types.LatestSigner(gspec.Config)
		)
		switch i {
		case 0:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil), signer, key)
		case 1:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil), signer, key)
		case 2:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil), signer, key)
		}
		if err != nil {
			t.Fatal(err)
		}
		block.AddTx(tx)
	})
	// account must exist pre eip 161
	blockchain, _ := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	if _, err := blockchain.InsertChain(types.Blocks{blocks[0]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); !st.Exist(theAddr) {
		t.Error("expected account to exist")
	}

	// account needs to be deleted post eip 161
	if _, err := blockchain.InsertChain(types.Blocks{blocks[1]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); st.Exist(theAddr) {
		t.Error("account should not exist")
	}

	// account mustn't be created post eip 161
	if _, err := blockchain.InsertChain(types.Blocks{blocks[2]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); st.Exist(theAddr) {
		t.Error("account should not exist")
	}
}

// This is a regression test (i.e. as weird as it is, don't delete it ever), which
// tests that under weird reorg conditions the blockchain and its internal header-
// chain return the same latest block/header.
//
// https://github.com/ethereum/go-ethereum/pull/15941
func TestBlockchainHeaderchainReorgConsistency(t *testing.T) {
	testBlockchainHeaderchainReorgConsistency(t, rawdb.HashScheme)
	testBlockchainHeaderchainReorgConsistency(t, rawdb.PathScheme)
}

func testBlockchainHeaderchainReorgConsistency(t *testing.T, scheme string) {
	// Generate a canonical chain to act as the main dataset
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	genDb, blocks, _ := GenerateChainWithGenesis(genesis, engine, 64, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// Generate a bunch of fork blocks, each side forking from the canonical chain
	forks := make([]*types.Block, len(blocks))
	for i := 0; i < len(forks); i++ {
		parent := genesis.ToBlock()
		if i > 0 {
			parent = blocks[i-1]
		}
		fork, _ := GenerateChain(genesis.Config, parent, engine, genDb, 1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
		forks[i] = fork[0]
	}
	// Import the canonical and fork chain side by side, verifying the current block
	// and current header consistency
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	for i := 0; i < len(blocks); i++ {
		if _, err := chain.InsertChain(blocks[i : i+1]); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", i, err)
		}
		if chain.CurrentBlock().Hash() != chain.CurrentHeader().Hash() {
			t.Errorf("block %d: current block/header mismatch: block #%d [%x..], header #%d [%x..]", i, chain.CurrentBlock().Number, chain.CurrentBlock().Hash().Bytes()[:4], chain.CurrentHeader().Number, chain.CurrentHeader().Hash().Bytes()[:4])
		}
		if _, err := chain.InsertChain(forks[i : i+1]); err != nil {
			t.Fatalf(" fork %d: failed to insert into chain: %v", i, err)
		}
		if chain.CurrentBlock().Hash() != chain.CurrentHeader().Hash() {
			t.Errorf(" fork %d: current block/header mismatch: block #%d [%x..], header #%d [%x..]", i, chain.CurrentBlock().Number, chain.CurrentBlock().Hash().Bytes()[:4], chain.CurrentHeader().Number, chain.CurrentHeader().Hash().Bytes()[:4])
		}
	}
}

// Tests that importing small side forks doesn't leave junk in the trie database
// cache (which would eventually cause memory issues).
func TestTrieForkGC(t *testing.T) {
	// Generate a canonical chain to act as the main dataset
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	genDb, blocks, _ := GenerateChainWithGenesis(genesis, engine, 2*TriesInMemory, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// Generate a bunch of fork blocks, each side forking from the canonical chain
	forks := make([]*types.Block, len(blocks))
	for i := 0; i < len(forks); i++ {
		parent := genesis.ToBlock()
		if i > 0 {
			parent = blocks[i-1]
		}
		fork, _ := GenerateChain(genesis.Config, parent, engine, genDb, 1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
		forks[i] = fork[0]
	}
	// Import the canonical and fork chain side by side, forcing the trie cache to cache both
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	for i := 0; i < len(blocks); i++ {
		if _, err := chain.InsertChain(blocks[i : i+1]); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", i, err)
		}
		if _, err := chain.InsertChain(forks[i : i+1]); err != nil {
			t.Fatalf("fork %d: failed to insert into chain: %v", i, err)
		}
	}
	// Dereference all the recent tries and ensure no past trie is left in
	for i := 0; i < TriesInMemory; i++ {
		chain.TrieDB().Dereference(blocks[len(blocks)-1-i].Root())
		chain.TrieDB().Dereference(forks[len(blocks)-1-i].Root())
	}
	if _, nodes, _, _ := chain.TrieDB().Size(); nodes > 0 { // all memory is returned in the nodes return for hashdb
		t.Fatalf("stale tries still alive after garbase collection")
	}
}

// Tests that doing large reorgs works even if the state associated with the
// forking point is not available any more.
func TestLargeReorgTrieGC(t *testing.T) {
	testLargeReorgTrieGC(t, rawdb.HashScheme)
	testLargeReorgTrieGC(t, rawdb.PathScheme)
}

func testLargeReorgTrieGC(t *testing.T, scheme string) {
	// Generate the original common chain segment and the two competing forks
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	genDb, shared, _ := GenerateChainWithGenesis(genesis, engine, 64, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })
	original, _ := GenerateChain(genesis.Config, shared[len(shared)-1], engine, genDb, 2*TriesInMemory, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
	competitor, _ := GenerateChain(genesis.Config, shared[len(shared)-1], engine, genDb, 2*TriesInMemory+1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{3}) })

	// Import the shared chain and the original canonical one
	db, _ := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	defer db.Close()

	chain, err := NewBlockChain(db, DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if _, err := chain.InsertChain(shared); err != nil {
		t.Fatalf("failed to insert shared chain: %v", err)
	}
	if _, err := chain.InsertChain(original); err != nil {
		t.Fatalf("failed to insert original chain: %v", err)
	}
	// Ensure that the state associated with the forking point is pruned away
	if chain.HasState(shared[len(shared)-1].Root()) {
		t.Fatalf("common-but-old ancestor still cache")
	}
	// Import the competitor chain without exceeding the canonical's TD and ensure
	// we have not processed any of the blocks (protection against malicious blocks)
	if _, err := chain.InsertChain(competitor[:len(competitor)-2]); err != nil {
		t.Fatalf("failed to insert competitor chain: %v", err)
	}
	for i, block := range competitor[:len(competitor)-2] {
		if chain.HasState(block.Root()) {
			t.Fatalf("competitor %d: low TD chain became processed", i)
		}
	}
	// Import the head of the competitor chain, triggering the reorg and ensure we
	// successfully reprocess all the stashed away blocks.
	if _, err := chain.InsertChain(competitor[len(competitor)-2:]); err != nil {
		t.Fatalf("failed to finalize competitor chain: %v", err)
	}
	// In path-based trie database implementation, it will keep 128 diff + 1 disk
	// layers, totally 129 latest states available. In hash-based it's 128.
	states := TestTriesInMemory
	if scheme == rawdb.PathScheme {
		states = states + 1
	}
	for i, block := range competitor[:len(competitor)-states] {
		if chain.HasState(block.Root()) {
			t.Fatalf("competitor %d: unexpected competing chain state", i)
		}
	}
	for i, block := range competitor[len(competitor)-states:] {
		if !chain.HasState(block.Root()) {
			t.Fatalf("competitor %d: competing chain state missing", i)
		}
	}
}

func TestBlockchainRecovery(t *testing.T) {
	testBlockchainRecovery(t, rawdb.HashScheme)
	testBlockchainRecovery(t, rawdb.PathScheme)
}

func testBlockchainRecovery(t *testing.T, scheme string) {
	// Configure and generate a sample block chain
	var (
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		gspec   = &Genesis{Config: params.TestChainConfig, Alloc: types.GenesisAlloc{address: {Balance: funds}}}
	)
	height := uint64(1024)
	_, blocks, receipts := GenerateChainWithGenesis(gspec, ethash.NewFaker(), int(height), nil)

	// Import the chain as a ancient-first node and ensure all pointers are updated
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	defer ancientDb.Close()
	ancient, _ := NewBlockChain(ancientDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := ancient.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	rawdb.WriteLastPivotNumber(ancientDb, blocks[len(blocks)-1].NumberU64()) // Force fast sync behavior
	ancient.Stop()

	// Destroy head fast block manually
	midBlock := blocks[len(blocks)/2]
	rawdb.WriteHeadFastBlockHash(ancientDb, midBlock.Hash())

	// Reopen broken blockchain again
	ancient, _ = NewBlockChain(ancientDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()
	if num := ancient.CurrentBlock().Number.Uint64(); num != 0 {
		t.Errorf("head block mismatch: have #%v, want #%v", num, 0)
	}
	if num := ancient.CurrentSnapBlock().Number.Uint64(); num != midBlock.NumberU64() {
		t.Errorf("head snap-block mismatch: have #%v, want #%v", num, midBlock.NumberU64())
	}
	if num := ancient.CurrentHeader().Number.Uint64(); num != midBlock.NumberU64() {
		t.Errorf("head header mismatch: have #%v, want #%v", num, midBlock.NumberU64())
	}
}

// This test checks that InsertReceiptChain will roll back correctly when attempting to insert a side chain.
func TestInsertReceiptChainRollback(t *testing.T) {
	testInsertReceiptChainRollback(t, rawdb.HashScheme)
	testInsertReceiptChainRollback(t, rawdb.PathScheme)
}

func testInsertReceiptChainRollback(t *testing.T, scheme string) {
	// Generate forked chain. The returned BlockChain object is used to process the side chain blocks.
	tmpChain, sideblocks, canonblocks, gspec, err := getLongAndShortChains(scheme)
	if err != nil {
		t.Fatal(err)
	}
	defer tmpChain.Stop()
	// Get the side chain receipts.
	if _, err := tmpChain.InsertChain(sideblocks); err != nil {
		t.Fatal("processing side chain failed:", err)
	}
	t.Log("sidechain head:", tmpChain.CurrentBlock().Number, tmpChain.CurrentBlock().Hash())
	sidechainReceipts := make([]types.Receipts, len(sideblocks))
	for i, block := range sideblocks {
		sidechainReceipts[i] = tmpChain.GetReceiptsByHash(block.Hash())
	}
	// Get the canon chain receipts.
	if _, err := tmpChain.InsertChain(canonblocks); err != nil {
		t.Fatal("processing canon chain failed:", err)
	}
	t.Log("canon head:", tmpChain.CurrentBlock().Number, tmpChain.CurrentBlock().Hash())
	canonReceipts := make([]types.Receipts, len(canonblocks))
	for i, block := range canonblocks {
		canonReceipts[i] = tmpChain.GetReceiptsByHash(block.Hash())
	}

	// Set up a BlockChain that uses the ancient store.
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	defer ancientDb.Close()

	ancientChain, _ := NewBlockChain(ancientDb, DefaultCacheConfigWithScheme(scheme), gspec, nil, ethash.NewFaker(), vm.Config{}, nil, nil)
	defer ancientChain.Stop()

	// Import the canonical header chain.
	canonHeaders := make([]*types.Header, len(canonblocks))
	for i, block := range canonblocks {
		canonHeaders[i] = block.Header()
	}
	if _, err = ancientChain.InsertHeaderChain(canonHeaders); err != nil {
		t.Fatal("can't import canon headers:", err)
	}

	// Try to insert blocks/receipts of the side chain.
	_, err = ancientChain.InsertReceiptChain(sideblocks, sidechainReceipts, uint64(len(sideblocks)))
	if err == nil {
		t.Fatal("expected error from InsertReceiptChain.")
	}
	if ancientChain.CurrentSnapBlock().Number.Uint64() != 0 {
		t.Fatalf("failed to rollback ancient data, want %d, have %d", 0, ancientChain.CurrentSnapBlock().Number)
	}
	if frozen, err := ancientChain.db.Ancients(); err != nil || frozen != 1 {
		t.Fatalf("failed to truncate ancient data, frozen index is %d", frozen)
	}

	// Insert blocks/receipts of the canonical chain.
	_, err = ancientChain.InsertReceiptChain(canonblocks, canonReceipts, uint64(len(canonblocks)))
	if err != nil {
		t.Fatalf("can't import canon chain receipts: %v", err)
	}
	if ancientChain.CurrentSnapBlock().Number.Uint64() != canonblocks[len(canonblocks)-1].NumberU64() {
		t.Fatalf("failed to insert ancient recept chain after rollback")
	}
	if frozen, _ := ancientChain.db.Ancients(); frozen != uint64(len(canonblocks))+1 {
		t.Fatalf("wrong ancients count %d", frozen)
	}
}

// Tests that importing a very large side fork, which is larger than the canon chain,
// but where the difficulty per block is kept low: this means that it will not
// overtake the 'canon' chain until after it's passed canon by about 200 blocks.
//
// Details at:
//   - https://github.com/ethereum/go-ethereum/issues/18977
//   - https://github.com/ethereum/go-ethereum/pull/18988
func TestLowDiffLongChain(t *testing.T) {
	testLowDiffLongChain(t, rawdb.HashScheme)
	testLowDiffLongChain(t, rawdb.PathScheme)
}

func testLowDiffLongChain(t *testing.T, scheme string) {
	// Generate a canonical chain to act as the main dataset
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	// We must use a pretty long chain to ensure that the fork doesn't overtake us
	// until after at least 128 blocks post tip
	genDb, blocks, _ := GenerateChainWithGenesis(genesis, engine, 6*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		b.OffsetTime(-9)
	})

	// Import the canonical chain
	diskdb, _ := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	defer diskdb.Close()

	chain, err := NewBlockChain(diskdb, DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	// Generate fork chain, starting from an early block
	parent := blocks[10]
	fork, _ := GenerateChain(genesis.Config, parent, engine, genDb, 8*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})
	})

	// And now import the fork
	if i, err := chain.InsertChain(fork); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", i, err)
	}
	head := chain.CurrentBlock()
	if got := fork[len(fork)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
	// Sanity check that all the canonical numbers are present
	header := chain.CurrentHeader()
	for number := head.Number.Uint64(); number > 0; number-- {
		if hash := chain.GetHeaderByNumber(number).Hash(); hash != header.Hash() {
			t.Fatalf("header %d: canonical hash mismatch: have %x, want %x", number, hash, header.Hash())
		}
		header = chain.GetHeader(header.ParentHash, number-1)
	}
}

// Tests that importing a sidechain (S), where
// - S is sidechain, containing blocks [Sn...Sm]
// - C is canon chain, containing blocks [G..Cn..Cm]
// - A common ancestor is placed at prune-point + blocksBetweenCommonAncestorAndPruneblock
// - The sidechain S is prepended with numCanonBlocksInSidechain blocks from the canon chain
//
// The mergePoint can be these values:
// -1: the transition won't happen
// 0:  the transition happens since genesis
// 1:  the transition happens after some chain segments
func testSideImport(t *testing.T, numCanonBlocksInSidechain, blocksBetweenCommonAncestorAndPruneblock int, mergePoint int) {
	// Generate a canonical chain to act as the main dataset
	chainConfig := *params.TestChainConfig
	var (
		merger = consensus.NewMerger(rawdb.NewMemoryDatabase())
		engine = beacon.New(ethash.NewFaker())
		key, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr   = crypto.PubkeyToAddress(key.PublicKey)
		nonce  = uint64(0)

		gspec = &Genesis{
			Config:  &chainConfig,
			Alloc:   types.GenesisAlloc{addr: {Balance: big.NewInt(math.MaxInt64)}},
			BaseFee: big.NewInt(params.InitialBaseFee),
		}
		signer     = types.LatestSigner(gspec.Config)
		mergeBlock = math.MaxInt32
	)
	// Generate and import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	// Activate the transition since genesis if required
	if mergePoint == 0 {
		mergeBlock = 0
		merger.ReachTTD()
		merger.FinalizePoS()

		// Set the terminal total difficulty in the config
		gspec.Config.TerminalTotalDifficulty = big.NewInt(0)
	}
	genDb, blocks, _ := GenerateChainWithGenesis(gspec, engine, 2*TriesInMemory, func(i int, gen *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(nonce, common.HexToAddress("deadbeef"), big.NewInt(100), 21000, big.NewInt(int64(i+1)*params.GWei), nil), signer, key)
		if err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}
		gen.AddTx(tx)
		if int(gen.header.Number.Uint64()) >= mergeBlock {
			gen.SetPoS()
		}
		nonce++
	})
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	lastPrunedIndex := len(blocks) - TestTriesInMemory - 1
	lastPrunedBlock := blocks[lastPrunedIndex-1]
	firstNonPrunedBlock := blocks[len(blocks)-TestTriesInMemory]

	// Verify pruning of lastPrunedBlock
	if chain.HasBlockAndState(lastPrunedBlock.Hash(), lastPrunedBlock.NumberU64()) {
		t.Errorf("Block %d not pruned", lastPrunedBlock.NumberU64())
	}
	// Verify firstNonPrunedBlock is not pruned
	if !chain.HasBlockAndState(firstNonPrunedBlock.Hash(), firstNonPrunedBlock.NumberU64()) {
		t.Errorf("Block %d pruned", firstNonPrunedBlock.NumberU64())
	}

	// Activate the transition in the middle of the chain
	if mergePoint == 1 {
		merger.ReachTTD()
		merger.FinalizePoS()
		// Set the terminal total difficulty in the config
		ttd := big.NewInt(int64(len(blocks)))
		ttd.Mul(ttd, params.GenesisDifficulty)
		gspec.Config.TerminalTotalDifficulty = ttd
		mergeBlock = len(blocks)
	}

	// Generate the sidechain
	// First block should be a known block, block after should be a pruned block. So
	// canon(pruned), side, side...

	// Generate fork chain, make it longer than canon
	parentIndex := lastPrunedIndex + blocksBetweenCommonAncestorAndPruneblock
	parent := blocks[parentIndex]
	fork, _ := GenerateChain(gspec.Config, parent, engine, genDb, 2*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})
		if int(b.header.Number.Uint64()) >= mergeBlock {
			b.SetPoS()
		}
	})
	// Prepend the parent(s)
	var sidechain []*types.Block
	for i := numCanonBlocksInSidechain; i > 0; i-- {
		sidechain = append(sidechain, blocks[parentIndex+1-i])
	}
	sidechain = append(sidechain, fork...)
	n, err := chain.InsertChain(sidechain)
	if err != nil {
		t.Errorf("Got error, %v number %d - %d", err, sidechain[n].NumberU64(), n)
	}
	head := chain.CurrentBlock()
	if got := fork[len(fork)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
}

// Tests that importing a sidechain (S), where
//   - S is sidechain, containing blocks [Sn...Sm]
//   - C is canon chain, containing blocks [G..Cn..Cm]
//   - The common ancestor Cc is pruned
//   - The first block in S: Sn, is == Cn
//
// That is: the sidechain for import contains some blocks already present in canon chain.
// So the blocks are:
//
//	[ Cn, Cn+1, Cc, Sn+3 ... Sm]
//	^    ^    ^  pruned
func TestPrunedImportSide(t *testing.T) {
	//glogger := log.NewGlogHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(false)))
	//glogger.Verbosity(3)
	//log.Root().SetHandler(log.Handler(glogger))
	testSideImport(t, 3, 3, -1)
	testSideImport(t, 3, -3, -1)
	testSideImport(t, 10, 0, -1)
	testSideImport(t, 1, 10, -1)
	testSideImport(t, 1, -10, -1)
}

func TestPrunedImportSideWithMerging(t *testing.T) {
	//glogger := log.NewGlogHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(false)))
	//glogger.Verbosity(3)
	//log.Root().SetHandler(log.Handler(glogger))
	testSideImport(t, 3, 3, 0)
	testSideImport(t, 3, -3, 0)
	testSideImport(t, 10, 0, 0)
	testSideImport(t, 1, 10, 0)
	testSideImport(t, 1, -10, 0)

	testSideImport(t, 3, 3, 1)
	testSideImport(t, 3, -3, 1)
	testSideImport(t, 10, 0, 1)
	testSideImport(t, 1, 10, 1)
	testSideImport(t, 1, -10, 1)
}

func TestInsertKnownHeaders(t *testing.T) {
	testInsertKnownChainData(t, "headers", rawdb.HashScheme)
	testInsertKnownChainData(t, "headers", rawdb.PathScheme)
}
func TestInsertKnownReceiptChain(t *testing.T) {
	testInsertKnownChainData(t, "receipts", rawdb.HashScheme)
	testInsertKnownChainData(t, "receipts", rawdb.PathScheme)
}
func TestInsertKnownBlocks(t *testing.T) {
	testInsertKnownChainData(t, "blocks", rawdb.HashScheme)
	testInsertKnownChainData(t, "blocks", rawdb.PathScheme)
}

func testInsertKnownChainData(t *testing.T, typ string, scheme string) {
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	genDb, blocks, receipts := GenerateChainWithGenesis(genesis, engine, 32, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// A longer chain but total difficulty is lower.
	blocks2, receipts2 := GenerateChain(genesis.Config, blocks[len(blocks)-1], engine, genDb, 65, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// A shorter chain but total difficulty is higher.
	blocks3, receipts3 := GenerateChain(genesis.Config, blocks[len(blocks)-1], engine, genDb, 64, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		b.OffsetTime(-9) // A higher difficulty
	})
	// Import the shared chain and the original canonical one
	chaindb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	defer chaindb.Close()

	chain, err := NewBlockChain(chaindb, DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	var (
		inserter func(blocks []*types.Block, receipts []types.Receipts) error
		asserter func(t *testing.T, block *types.Block)
	)
	if typ == "headers" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			_, err := chain.InsertHeaderChain(headers)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentHeader().Hash() != block.Hash() {
				t.Fatalf("current head header mismatch, have %v, want %v", chain.CurrentHeader().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else if typ == "receipts" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			_, err := chain.InsertHeaderChain(headers)
			if err != nil {
				return err
			}
			_, err = chain.InsertReceiptChain(blocks, receipts, 0)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentSnapBlock().Hash() != block.Hash() {
				t.Fatalf("current head fast block mismatch, have %v, want %v", chain.CurrentSnapBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			_, err := chain.InsertChain(blocks)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentBlock().Hash() != block.Hash() {
				t.Fatalf("current head block mismatch, have %v, want %v", chain.CurrentBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	}

	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}

	// Reimport the chain data again. All the imported
	// chain data are regarded "known" data.
	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks[len(blocks)-1])

	// Import a long canonical chain with some known data as prefix.
	rollback := blocks[len(blocks)/2].NumberU64()

	chain.SetHead(rollback - 1)
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])

	// Import a heavier shorter but higher total difficulty chain with some known data as prefix.
	if err := inserter(append(blocks, blocks3...), append(receipts, receipts3...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks3[len(blocks3)-1])

	// Import a longer but lower total difficulty chain with some known data as prefix.
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	// The head shouldn't change.
	asserter(t, blocks3[len(blocks3)-1])

	// Rollback the heavier chain and re-insert the longer chain again
	chain.SetHead(rollback - 1)
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])
}

func TestInsertKnownHeadersWithMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "headers", 0)
}
func TestInsertKnownReceiptChainWithMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "receipts", 0)
}
func TestInsertKnownBlocksWithMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "blocks", 0)
}
func TestInsertKnownHeadersAfterMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "headers", 1)
}
func TestInsertKnownReceiptChainAfterMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "receipts", 1)
}
func TestInsertKnownBlocksAfterMerging(t *testing.T) {
	testInsertKnownChainDataWithMerging(t, "blocks", 1)
}

// mergeHeight can be assigned in these values:
// 0: means the merging is applied since genesis
// 1: means the merging is applied after the first segment
func testInsertKnownChainDataWithMerging(t *testing.T, typ string, mergeHeight int) {
	// Copy the TestChainConfig so we can modify it during tests
	chainConfig := *params.TestChainConfig
	var (
		genesis = &Genesis{
			BaseFee: big.NewInt(params.InitialBaseFee),
			Config:  &chainConfig,
		}
		engine     = beacon.New(ethash.NewFaker())
		mergeBlock = uint64(math.MaxUint64)
	)
	// Apply merging since genesis
	if mergeHeight == 0 {
		genesis.Config.TerminalTotalDifficulty = big.NewInt(0)
		mergeBlock = uint64(0)
	}

	genDb, blocks, receipts := GenerateChainWithGenesis(genesis, engine, 32,
		func(i int, b *BlockGen) {
			if b.header.Number.Uint64() >= mergeBlock {
				b.SetPoS()
			}
			b.SetCoinbase(common.Address{1})
		})

	// Apply merging after the first segment
	if mergeHeight == 1 {
		// TTD is genesis diff + blocks
		ttd := big.NewInt(1 + int64(len(blocks)))
		ttd.Mul(ttd, params.GenesisDifficulty)
		genesis.Config.TerminalTotalDifficulty = ttd
		mergeBlock = uint64(len(blocks))
	}
	// Longer chain and shorter chain
	blocks2, receipts2 := GenerateChain(genesis.Config, blocks[len(blocks)-1], engine, genDb, 65, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		if b.header.Number.Uint64() >= mergeBlock {
			b.SetPoS()
		}
	})
	blocks3, receipts3 := GenerateChain(genesis.Config, blocks[len(blocks)-1], engine, genDb, 64, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		b.OffsetTime(-9) // Time shifted, difficulty shouldn't be changed
		if b.header.Number.Uint64() >= mergeBlock {
			b.SetPoS()
		}
	})
	// Import the shared chain and the original canonical one
	chaindb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	defer chaindb.Close()

	chain, err := NewBlockChain(chaindb, nil, genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	var (
		inserter func(blocks []*types.Block, receipts []types.Receipts) error
		asserter func(t *testing.T, block *types.Block)
	)
	if typ == "headers" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			i, err := chain.InsertHeaderChain(headers)
			if err != nil {
				return fmt.Errorf("index %d, number %d: %w", i, headers[i].Number, err)
			}
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentHeader().Hash() != block.Hash() {
				t.Fatalf("current head header mismatch, have %v, want %v", chain.CurrentHeader().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else if typ == "receipts" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			i, err := chain.InsertHeaderChain(headers)
			if err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
			_, err = chain.InsertReceiptChain(blocks, receipts, 0)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentSnapBlock().Hash() != block.Hash() {
				t.Fatalf("current head fast block mismatch, have %v, want %v", chain.CurrentSnapBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			i, err := chain.InsertChain(blocks)
			if err != nil {
				return fmt.Errorf("index %d: %w", i, err)
			}
			return nil
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentBlock().Hash() != block.Hash() {
				t.Fatalf("current head block mismatch, have %v, want %v", chain.CurrentBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	}
	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}

	// Reimport the chain data again. All the imported
	// chain data are regarded "known" data.
	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks[len(blocks)-1])

	// Import a long canonical chain with some known data as prefix.
	rollback := blocks[len(blocks)/2].NumberU64()
	chain.SetHead(rollback - 1)
	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks[len(blocks)-1])

	// Import a longer chain with some known data as prefix.
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])

	// Import a shorter chain with some known data as prefix.
	// The reorg is expected since the fork choice rule is
	// already changed.
	if err := inserter(append(blocks, blocks3...), append(receipts, receipts3...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	// The head shouldn't change.
	asserter(t, blocks3[len(blocks3)-1])

	// Reimport the longer chain again, the reorg is still expected
	chain.SetHead(rollback - 1)
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])
}

// getLongAndShortChains returns two chains: A is longer, B is heavier.
func getLongAndShortChains(scheme string) (*BlockChain, []*types.Block, []*types.Block, *Genesis, error) {
	// Generate a canonical chain to act as the main dataset
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	// Generate and import the canonical chain,
	// Offset the time, to keep the difficulty low
	genDb, longChain, _ := GenerateChainWithGenesis(genesis, engine, 80, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
	})
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to create tester chain: %v", err)
	}
	// Generate fork chain, make it shorter than canon, with common ancestor pretty early
	parentIndex := 3
	parent := longChain[parentIndex]
	heavyChainExt, _ := GenerateChain(genesis.Config, parent, engine, genDb, 75, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})
		b.OffsetTime(-9)
	})
	var heavyChain []*types.Block
	heavyChain = append(heavyChain, longChain[:parentIndex+1]...)
	heavyChain = append(heavyChain, heavyChainExt...)

	// Verify that the test is sane
	var (
		longerTd  = new(big.Int)
		shorterTd = new(big.Int)
	)
	for index, b := range longChain {
		longerTd.Add(longerTd, b.Difficulty())
		if index <= parentIndex {
			shorterTd.Add(shorterTd, b.Difficulty())
		}
	}
	for _, b := range heavyChain {
		shorterTd.Add(shorterTd, b.Difficulty())
	}
	if shorterTd.Cmp(longerTd) <= 0 {
		return nil, nil, nil, nil, fmt.Errorf("test is moot, heavyChain td (%v) must be larger than canon td (%v)", shorterTd, longerTd)
	}
	longerNum := longChain[len(longChain)-1].NumberU64()
	shorterNum := heavyChain[len(heavyChain)-1].NumberU64()
	if shorterNum >= longerNum {
		return nil, nil, nil, nil, fmt.Errorf("test is moot, heavyChain num (%v) must be lower than canon num (%v)", shorterNum, longerNum)
	}
	return chain, longChain, heavyChain, genesis, nil
}

// TestReorgToShorterRemovesCanonMapping tests that if we
// 1. Have a chain [0 ... N .. X]
// 2. Reorg to shorter but heavier chain [0 ... N ... Y]
// 3. Then there should be no canon mapping for the block at height X
// 4. The forked block should still be retrievable by hash
func TestReorgToShorterRemovesCanonMapping(t *testing.T) {
	testReorgToShorterRemovesCanonMapping(t, rawdb.HashScheme)
	testReorgToShorterRemovesCanonMapping(t, rawdb.PathScheme)
}

func testReorgToShorterRemovesCanonMapping(t *testing.T, scheme string) {
	chain, canonblocks, sideblocks, _, err := getLongAndShortChains(scheme)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(canonblocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	canonNum := chain.CurrentBlock().Number.Uint64()
	canonHash := chain.CurrentBlock().Hash()
	_, err = chain.InsertChain(sideblocks)
	if err != nil {
		t.Errorf("Got error, %v", err)
	}
	head := chain.CurrentBlock()
	if got := sideblocks[len(sideblocks)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
	// We have now inserted a sidechain.
	if blockByNum := chain.GetBlockByNumber(canonNum); blockByNum != nil {
		t.Errorf("expected block to be gone: %v", blockByNum.NumberU64())
	}
	if headerByNum := chain.GetHeaderByNumber(canonNum); headerByNum != nil {
		t.Errorf("expected header to be gone: %v", headerByNum.Number)
	}
	if blockByHash := chain.GetBlockByHash(canonHash); blockByHash == nil {
		t.Errorf("expected block to be present: %x", blockByHash.Hash())
	}
	if headerByHash := chain.GetHeaderByHash(canonHash); headerByHash == nil {
		t.Errorf("expected header to be present: %x", headerByHash.Hash())
	}
}

// TestReorgToShorterRemovesCanonMappingHeaderChain is the same scenario
// as TestReorgToShorterRemovesCanonMapping, but applied on headerchain
// imports -- that is, for fast sync
func TestReorgToShorterRemovesCanonMappingHeaderChain(t *testing.T) {
	testReorgToShorterRemovesCanonMappingHeaderChain(t, rawdb.HashScheme)
	testReorgToShorterRemovesCanonMappingHeaderChain(t, rawdb.PathScheme)
}

func testReorgToShorterRemovesCanonMappingHeaderChain(t *testing.T, scheme string) {
	chain, canonblocks, sideblocks, _, err := getLongAndShortChains(scheme)
	if err != nil {
		t.Fatal(err)
	}
	defer chain.Stop()

	// Convert into headers
	canonHeaders := make([]*types.Header, len(canonblocks))
	for i, block := range canonblocks {
		canonHeaders[i] = block.Header()
	}
	if n, err := chain.InsertHeaderChain(canonHeaders); err != nil {
		t.Fatalf("header %d: failed to insert into chain: %v", n, err)
	}
	canonNum := chain.CurrentHeader().Number.Uint64()
	canonHash := chain.CurrentBlock().Hash()
	sideHeaders := make([]*types.Header, len(sideblocks))
	for i, block := range sideblocks {
		sideHeaders[i] = block.Header()
	}
	if n, err := chain.InsertHeaderChain(sideHeaders); err != nil {
		t.Fatalf("header %d: failed to insert into chain: %v", n, err)
	}
	head := chain.CurrentHeader()
	if got := sideblocks[len(sideblocks)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
	// We have now inserted a sidechain.
	if blockByNum := chain.GetBlockByNumber(canonNum); blockByNum != nil {
		t.Errorf("expected block to be gone: %v", blockByNum.NumberU64())
	}
	if headerByNum := chain.GetHeaderByNumber(canonNum); headerByNum != nil {
		t.Errorf("expected header to be gone: %v", headerByNum.Number.Uint64())
	}
	if blockByHash := chain.GetBlockByHash(canonHash); blockByHash == nil {
		t.Errorf("expected block to be present: %x", blockByHash.Hash())
	}
	if headerByHash := chain.GetHeaderByHash(canonHash); headerByHash == nil {
		t.Errorf("expected header to be present: %x", headerByHash.Hash())
	}
}

// Benchmarks large blocks with value transfers to non-existing accounts
func benchmarkLargeNumberOfValueToNonexisting(b *testing.B, numTxs, numBlocks int, recipientFn func(uint64) common.Address, dataFn func(uint64) []byte) {
	var (
		signer          = types.HomesteadSigner{}
		testBankKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
		bankFunds       = big.NewInt(100000000000000000)
		gspec           = &Genesis{
			Config: params.TestChainConfig,
			Alloc: types.GenesisAlloc{
				testBankAddress: {Balance: bankFunds},
				common.HexToAddress("0xc0de"): {
					Code:    []byte{0x60, 0x01, 0x50},
					Balance: big.NewInt(0),
				}, // push 1, pop
			},
			GasLimit: 100e6, // 100 M
		}
	)
	// Generate the original common chain segment and the two competing forks
	engine := ethash.NewFaker()

	blockGenerator := func(i int, block *BlockGen) {
		block.SetCoinbase(common.Address{1})
		for txi := 0; txi < numTxs; txi++ {
			uniq := uint64(i*numTxs + txi)
			recipient := recipientFn(uniq)
			tx, err := types.SignTx(types.NewTransaction(uniq, recipient, big.NewInt(1), params.TxGas, block.header.BaseFee, nil), signer, testBankKey)
			if err != nil {
				b.Error(err)
			}
			block.AddTx(tx)
		}
	}

	_, shared, _ := GenerateChainWithGenesis(gspec, engine, numBlocks, blockGenerator)
	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Import the shared chain and the original canonical one
		chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{}, nil, nil)
		if err != nil {
			b.Fatalf("failed to create tester chain: %v", err)
		}
		b.StartTimer()
		if _, err := chain.InsertChain(shared); err != nil {
			b.Fatalf("failed to insert shared chain: %v", err)
		}
		b.StopTimer()
		block := chain.GetBlockByHash(chain.CurrentBlock().Hash())
		if got := block.Transactions().Len(); got != numTxs*numBlocks {
			b.Fatalf("Transactions were not included, expected %d, got %d", numTxs*numBlocks, got)
		}
	}
}

func BenchmarkBlockChain_1x1000ValueTransferToNonexisting(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(new(big.Int).SetUint64(1337 + nonce))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

func BenchmarkBlockChain_1x1000ValueTransferToExisting(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	b.StopTimer()
	b.ResetTimer()

	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(new(big.Int).SetUint64(1337))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

func BenchmarkBlockChain_1x1000Executions(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	b.StopTimer()
	b.ResetTimer()

	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(new(big.Int).SetUint64(0xc0de))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

// Tests that importing a some old blocks, where all blocks are before the
// pruning point.
// This internally leads to a sidechain import, since the blocks trigger an
// ErrPrunedAncestor error.
// This may e.g. happen if
//  1. Downloader rollbacks a batch of inserted blocks and exits
//  2. Downloader starts to sync again
//  3. The blocks fetched are all known and canonical blocks
func TestSideImportPrunedBlocks(t *testing.T) {
	testSideImportPrunedBlocks(t, rawdb.HashScheme)
	testSideImportPrunedBlocks(t, rawdb.PathScheme)
}

func testSideImportPrunedBlocks(t *testing.T, scheme string) {
	// Generate a canonical chain to act as the main dataset
	engine := ethash.NewFaker()
	genesis := &Genesis{
		Config:  params.TestChainConfig,
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	// Generate and import the canonical chain
	_, blocks, _ := GenerateChainWithGenesis(genesis, engine, 2*TriesInMemory, nil)

	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), genesis, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	// In path-based trie database implementation, it will keep 128 diff + 1 disk
	// layers, totally 129 latest states available. In hash-based it's 128.
	states := TestTriesInMemory
	if scheme == rawdb.PathScheme {
		states = TestTriesInMemory + 1
	}
	lastPrunedIndex := len(blocks) - states - 1
	lastPrunedBlock := blocks[lastPrunedIndex]

	// Verify pruning of lastPrunedBlock
	if chain.HasBlockAndState(lastPrunedBlock.Hash(), lastPrunedBlock.NumberU64()) {
		t.Errorf("Block %d not pruned", lastPrunedBlock.NumberU64())
	}
	firstNonPrunedBlock := blocks[len(blocks)-states]
	// Verify firstNonPrunedBlock is not pruned
	if !chain.HasBlockAndState(firstNonPrunedBlock.Hash(), firstNonPrunedBlock.NumberU64()) {
		t.Errorf("Block %d pruned", firstNonPrunedBlock.NumberU64())
	}
	// Now re-import some old blocks
	blockToReimport := blocks[5:8]
	_, err = chain.InsertChain(blockToReimport)
	if err != nil {
		t.Errorf("Got error, %v", err)
	}
}

// TestDeleteCreateRevert tests a weird state transition corner case that we hit
// while changing the internals of statedb. The workflow is that a contract is
// self destructed, then in a followup transaction (but same block) it's created
// again and the transaction reverted.
//
// The original statedb implementation flushed dirty objects to the tries after
// each transaction, so this works ok. The rework accumulated writes in memory
// first, but the journal wiped the entire state object on create-revert.
func TestDeleteCreateRevert(t *testing.T) {
	testDeleteCreateRevert(t, rawdb.HashScheme)
	testDeleteCreateRevert(t, rawdb.PathScheme)
}

func testDeleteCreateRevert(t *testing.T, scheme string) {
	var (
		aa     = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
		bb     = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(100000000000000000)
		gspec   = &Genesis{
			Config: params.TestChainConfig,
			Alloc: types.GenesisAlloc{
				address: {Balance: funds},
				// The address 0xAAAAA selfdestructs if called
				aa: {
					// Code needs to just selfdestruct
					Code:    []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)},
					Nonce:   1,
					Balance: big.NewInt(0),
				},
				// The address 0xBBBB send 1 wei to 0xAAAA, then reverts
				bb: {
					Code: []byte{
						byte(vm.PC),          // [0]
						byte(vm.DUP1),        // [0,0]
						byte(vm.DUP1),        // [0,0,0]
						byte(vm.DUP1),        // [0,0,0,0]
						byte(vm.PUSH1), 0x01, // [0,0,0,0,1] (value)
						byte(vm.PUSH2), 0xaa, 0xaa, // [0,0,0,0,1, 0xaaaa]
						byte(vm.GAS),
						byte(vm.CALL),
						byte(vm.REVERT),
					},
					Balance: big.NewInt(1),
				},
			},
		}
	)

	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AAAA
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		// One transaction to BBBB
		tx, _ = types.SignTx(types.NewTransaction(1, bb,
			big.NewInt(0), 100000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
}

// TestDeleteRecreateSlots tests a state-transition that contains both deletion
// and recreation of contract state.
// Contract A exists, has slots 1 and 2 set
// Tx 1: Selfdestruct A
// Tx 2: Re-create A, set slots 3 and 4
// Expected outcome is that _all_ slots are cleared from A, due to the selfdestruct,
// and then the new slots exist
func TestDeleteRecreateSlots(t *testing.T) {
	testDeleteRecreateSlots(t, rawdb.HashScheme)
	testDeleteRecreateSlots(t, rawdb.PathScheme)
}

func testDeleteRecreateSlots(t *testing.T, scheme string) {
	var (
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address   = crypto.PubkeyToAddress(key.PublicKey)
		funds     = big.NewInt(1000000000000000)
		bb        = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. Set slots 3=3, 4=4,
	// 2. Return aaCode

	initCode := []byte{
		byte(vm.PUSH1), 0x3, // value
		byte(vm.PUSH1), 0x3, // location
		byte(vm.SSTORE),     // Set slot[3] = 3
		byte(vm.PUSH1), 0x4, // value
		byte(vm.PUSH1), 0x4, // location
		byte(vm.SSTORE), // Set slot[4] = 4
		// Slots are set, now return the code
		byte(vm.PUSH2), byte(vm.PC), byte(vm.SELFDESTRUCT), // Push code on stack
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		// Code is now in memory.
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), byte(32 - 2), // offset
		byte(vm.RETURN),
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
			// The contract BB recreates AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AA, to kill it
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		// One transaction to BB, to recreate AA
		tx, _ = types.SignTx(types.NewTransaction(1, bb,
			big.NewInt(0), 100000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{
		Tracer: logger.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	statedb, _ := chain.State()

	// If all is correct, then slot 1 and 2 are zero
	if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	// Also, 3 and 4 should be set
	if got, exp := statedb.GetState(aa, common.HexToHash("03")), common.HexToHash("03"); got != exp {
		t.Fatalf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("04")), common.HexToHash("04"); got != exp {
		t.Fatalf("got %x exp %x", got, exp)
	}
}

// TestDeleteRecreateAccount tests a state-transition that contains deletion of a
// contract with storage, and a recreate of the same contract via a
// regular value-transfer
// Expected outcome is that _all_ slots are cleared from A
func TestDeleteRecreateAccount(t *testing.T) {
	testDeleteRecreateAccount(t, rawdb.HashScheme)
	testDeleteRecreateAccount(t, rawdb.PathScheme)
}

func testDeleteRecreateAccount(t *testing.T, scheme string) {
	var (
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)

		aa        = common.HexToAddress("0x7217d81b76bdd8707601e959454e3d776aee5f43")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
		},
	}

	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AA, to kill it
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		// One transaction to AA, to recreate it (but without storage
		tx, _ = types.SignTx(types.NewTransaction(1, aa,
			big.NewInt(1), 100000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{
		Tracer: logger.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	statedb, _ := chain.State()

	// If all is correct, then both slots are zero
	if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
}

// TestDeleteRecreateSlotsAcrossManyBlocks tests multiple state-transition that contains both deletion
// and recreation of contract state.
// Contract A exists, has slots 1 and 2 set
// Tx 1: Selfdestruct A
// Tx 2: Re-create A, set slots 3 and 4
// Expected outcome is that _all_ slots are cleared from A, due to the selfdestruct,
// and then the new slots exist
func TestDeleteRecreateSlotsAcrossManyBlocks(t *testing.T) {
	testDeleteRecreateSlotsAcrossManyBlocks(t, rawdb.HashScheme)
	testDeleteRecreateSlotsAcrossManyBlocks(t, rawdb.PathScheme)
}

func testDeleteRecreateSlotsAcrossManyBlocks(t *testing.T, scheme string) {
	var (
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address   = crypto.PubkeyToAddress(key.PublicKey)
		funds     = big.NewInt(1000000000000000)
		bb        = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. Set slots 3=blocknum+1, 4=4,
	// 2. Return aaCode

	initCode := []byte{
		byte(vm.PUSH1), 0x1, //
		byte(vm.NUMBER),     // value = number + 1
		byte(vm.ADD),        //
		byte(vm.PUSH1), 0x3, // location
		byte(vm.SSTORE),     // Set slot[3] = number + 1
		byte(vm.PUSH1), 0x4, // value
		byte(vm.PUSH1), 0x4, // location
		byte(vm.SSTORE), // Set slot[4] = 4
		// Slots are set, now return the code
		byte(vm.PUSH2), byte(vm.PC), byte(vm.SELFDESTRUCT), // Push code on stack
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		// Code is now in memory.
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), byte(32 - 2), // offset
		byte(vm.RETURN),
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
			// The contract BB recreates AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	var nonce uint64

	type expectation struct {
		exist    bool
		blocknum int
		values   map[int]int
	}
	var current = &expectation{
		exist:    true, // exists in genesis
		blocknum: 0,
		values:   map[int]int{1: 1, 2: 2},
	}
	var expectations []*expectation
	var newDestruct = func(e *expectation, b *BlockGen) *types.Transaction {
		tx, _ := types.SignTx(types.NewTransaction(nonce, aa,
			big.NewInt(0), 50000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		nonce++
		if e.exist {
			e.exist = false
			e.values = nil
		}
		//t.Logf("block %d; adding destruct\n", e.blocknum)
		return tx
	}
	var newResurrect = func(e *expectation, b *BlockGen) *types.Transaction {
		tx, _ := types.SignTx(types.NewTransaction(nonce, bb,
			big.NewInt(0), 100000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		nonce++
		if !e.exist {
			e.exist = true
			e.values = map[int]int{3: e.blocknum + 1, 4: 4}
		}
		//t.Logf("block %d; adding resurrect\n", e.blocknum)
		return tx
	}

	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 150, func(i int, b *BlockGen) {
		var exp = new(expectation)
		exp.blocknum = i + 1
		exp.values = make(map[int]int)
		for k, v := range current.values {
			exp.values[k] = v
		}
		exp.exist = current.exist

		b.SetCoinbase(common.Address{1})
		if i%2 == 0 {
			b.AddTx(newDestruct(exp, b))
		}
		if i%3 == 0 {
			b.AddTx(newResurrect(exp, b))
		}
		if i%5 == 0 {
			b.AddTx(newDestruct(exp, b))
		}
		if i%7 == 0 {
			b.AddTx(newResurrect(exp, b))
		}
		expectations = append(expectations, exp)
		current = exp
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{
		//Debug:  true,
		//Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	var asHash = func(num int) common.Hash {
		return common.BytesToHash([]byte{byte(num)})
	}
	for i, block := range blocks {
		blockNum := i + 1
		if n, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", n, err)
		}
		statedb, _ := chain.State()
		// If all is correct, then slot 1 and 2 are zero
		if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
			t.Errorf("block %d, got %x exp %x", blockNum, got, exp)
		}
		if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
			t.Errorf("block %d, got %x exp %x", blockNum, got, exp)
		}
		exp := expectations[i]
		if exp.exist {
			if !statedb.Exist(aa) {
				t.Fatalf("block %d, expected %v to exist, it did not", blockNum, aa)
			}
			for slot, val := range exp.values {
				if gotValue, expValue := statedb.GetState(aa, asHash(slot)), asHash(val); gotValue != expValue {
					t.Fatalf("block %d, slot %d, got %x exp %x", blockNum, slot, gotValue, expValue)
				}
			}
		} else {
			if statedb.Exist(aa) {
				t.Fatalf("block %d, expected %v to not exist, it did", blockNum, aa)
			}
		}
	}
}

// TestInitThenFailCreateContract tests a pretty notorious case that happened
// on mainnet over blocks 7338108, 7338110 and 7338115.
//   - Block 7338108: address e771789f5cccac282f23bb7add5690e1f6ca467c is initiated
//     with 0.001 ether (thus created but no code)
//   - Block 7338110: a CREATE2 is attempted. The CREATE2 would deploy code on
//     the same address e771789f5cccac282f23bb7add5690e1f6ca467c. However, the
//     deployment fails due to OOG during initcode execution
//   - Block 7338115: another tx checks the balance of
//     e771789f5cccac282f23bb7add5690e1f6ca467c, and the snapshotter returned it as
//     zero.
//
// The problem being that the snapshotter maintains a destructset, and adds items
// to the destructset in case something is created "onto" an existing item.
// We need to either roll back the snapDestructs, or not place it into snapDestructs
// in the first place.
//

func TestInitThenFailCreateContract(t *testing.T) {
	testInitThenFailCreateContract(t, rawdb.HashScheme)
	testInitThenFailCreateContract(t, rawdb.PathScheme)
}

func testInitThenFailCreateContract(t *testing.T, scheme string) {
	var (
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		bb      = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
	)

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. If blocknum < 1, error out (e.g invalid opcode)
	// 2. else, return a snippet of code
	initCode := []byte{
		byte(vm.PUSH1), 0x1, // y (2)
		byte(vm.NUMBER), // x (number)
		byte(vm.GT),     // x > y?
		byte(vm.PUSH1), byte(0x8),
		byte(vm.JUMPI), // jump to label if number > 2
		byte(0xFE),     // illegal opcode
		byte(vm.JUMPDEST),
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.RETURN), // return 2 bytes of zero-code
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
			// The address aa has some funds
			aa: {Balance: big.NewInt(100000)},
			// The contract BB tries to create code onto AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	nonce := uint64(0)
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 4, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to BB
		tx, _ := types.SignTx(types.NewTransaction(nonce, bb,
			big.NewInt(0), 100000, b.header.BaseFee, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		nonce++
	})

	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{
		//Debug:  true,
		//Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	statedb, _ := chain.State()
	if got, exp := statedb.GetBalance(aa), uint256.NewInt(100000); got.Cmp(exp) != 0 {
		t.Fatalf("Genesis err, got %v exp %v", got, exp)
	}
	// First block tries to create, but fails
	{
		block := blocks[0]
		if _, err := chain.InsertChain([]*types.Block{blocks[0]}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
		statedb, _ = chain.State()
		if got, exp := statedb.GetBalance(aa), uint256.NewInt(100000); got.Cmp(exp) != 0 {
			t.Fatalf("block %d: got %v exp %v", block.NumberU64(), got, exp)
		}
	}
	// Import the rest of the blocks
	for _, block := range blocks[1:] {
		if _, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
	}
}

// TestEIP2718Transition* tests that an EIP-2718 transaction will be accepted
// after the fork block has passed. This is verified by sending an EIP-2930
// access list transaction, which specifies a single slot access, and then
// checking that the gas usage of a hot SLOAD and a cold SLOAD are calculated
// correctly.

// TestEIP2718TransitionWithTestChainConfig tests EIP-2718 with TestChainConfig.
func TestEIP2718TransitionWithTestChainConfig(t *testing.T) {
	testEIP2718TransitionWithConfig(t, rawdb.HashScheme, params.TestChainConfig)
	testEIP2718TransitionWithConfig(t, rawdb.HashScheme, params.TestChainConfig)
}

func preShanghaiConfig() *params.ChainConfig {
	config := *params.SatoshiTestChainConfig
	config.ShanghaiTime = nil
	config.KeplerTime = nil
	config.DemeterTime = nil
	config.AthenaTime = nil
	config.CancunTime = nil
	return &config
}

// TestEIP2718TransitionWithSatoshiConfig tests EIP-2718 with Satoshi Config.
func TestEIP2718TransitionWithSatoshiConfig(t *testing.T) {
	testEIP2718TransitionWithConfig(t, rawdb.HashScheme, preShanghaiConfig())
	testEIP2718TransitionWithConfig(t, rawdb.PathScheme, preShanghaiConfig())
}

// testEIP2718TransitionWithConfig tests EIP02718 with given ChainConfig.
func testEIP2718TransitionWithConfig(t *testing.T, scheme string, config *params.ChainConfig) {
	var (
		aa     = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		gspec   = &Genesis{
			Config: config,
			Alloc: types.GenesisAlloc{
				address: {Balance: funds},
				// The address 0xAAAA sloads 0x00 and 0x01
				aa: {
					Code: []byte{
						byte(vm.PC),
						byte(vm.PC),
						byte(vm.SLOAD),
						byte(vm.SLOAD),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
			},
		}
	)
	// Generate blocks
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})

		// One transaction to 0xAAAA
		signer := types.LatestSigner(gspec.Config)
		tx, _ := types.SignNewTx(key, signer, &types.AccessListTx{
			ChainID:  gspec.Config.ChainID,
			Nonce:    0,
			To:       &aa,
			Gas:      30000,
			GasPrice: b.header.BaseFee,
			AccessList: types.AccessList{{
				Address:     aa,
				StorageKeys: []common.Hash{{0}},
			}},
		})
		b.AddTx(tx)
	})

	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block := chain.GetBlockByNumber(1)

	// Expected gas is intrinsic + 2 * pc + hot load + cold load, since only one load is in the access list
	expected := params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas +
		vm.GasQuickStep*2 + params.WarmStorageReadCostEIP2929 + params.ColdSloadCostEIP2929
	if block.GasUsed() != expected {
		t.Fatalf("incorrect amount of gas spent: expected %d, got %d", expected, block.GasUsed())
	}
}

// TestEIP1559Transition tests the following:
//
//  1. A transaction whose gasFeeCap is greater than the baseFee is valid.
//  2. Gas accounting for access lists on EIP-1559 transactions is correct.
//  3. Only the transaction's tip will be received by the coinbase.
//  4. The transaction sender pays for both the tip and baseFee.
//  5. The coinbase receives only the partially realized tip when
//     gasFeeCap - gasTipCap < baseFee.
//  6. Legacy transaction behave as expected (e.g. gasPrice = gasFeeCap = gasTipCap).
func TestEIP1559Transition(t *testing.T) {
	testEIP1559Transition(t, rawdb.HashScheme)
	testEIP1559Transition(t, rawdb.PathScheme)
}

func testEIP1559Transition(t *testing.T, scheme string) {
	var (
		aa     = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
		engine = ethash.NewFaker()

		// A sender who makes transactions, has some funds
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		funds   = new(big.Int).Mul(common.Big1, big.NewInt(params.Ether))
		config  = *params.AllEthashProtocolChanges
		gspec   = &Genesis{
			Config: &config,
			Alloc: types.GenesisAlloc{
				addr1: {Balance: funds},
				addr2: {Balance: funds},
				// The address 0xAAAA sloads 0x00 and 0x01
				aa: {
					Code: []byte{
						byte(vm.PC),
						byte(vm.PC),
						byte(vm.SLOAD),
						byte(vm.SLOAD),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
			},
		}
	)

	gspec.Config.BerlinBlock = common.Big0
	gspec.Config.LondonBlock = common.Big0
	signer := types.LatestSigner(gspec.Config)

	genDb, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})

		// One transaction to 0xAAAA
		accesses := types.AccessList{types.AccessTuple{
			Address:     aa,
			StorageKeys: []common.Hash{{0}},
		}}

		txdata := &types.DynamicFeeTx{
			ChainID:    gspec.Config.ChainID,
			Nonce:      0,
			To:         &aa,
			Gas:        30000,
			GasFeeCap:  newGwei(5),
			GasTipCap:  big.NewInt(2),
			AccessList: accesses,
			Data:       []byte{},
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key1)

		b.AddTx(tx)
	})
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block := chain.GetBlockByNumber(1)

	// 1+2: Ensure EIP-1559 access lists are accounted for via gas usage.
	expectedGas := params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas +
		vm.GasQuickStep*2 + params.WarmStorageReadCostEIP2929 + params.ColdSloadCostEIP2929
	if block.GasUsed() != expectedGas {
		t.Fatalf("incorrect amount of gas spent: expected %d, got %d", expectedGas, block.GasUsed())
	}

	state, _ := chain.State()

	// 3: Ensure that miner received only the tx's tip.
	actual := state.GetBalance(block.Coinbase()).ToBig()
	expected := new(big.Int).Add(
		new(big.Int).SetUint64(block.GasUsed()*block.Transactions()[0].GasTipCap().Uint64()),
		ethash.ConstantinopleBlockReward.ToBig(),
	)
	if actual.Cmp(expected) != 0 {
		t.Fatalf("miner balance incorrect: expected %d, got %d", expected, actual)
	}

	// 4: Ensure the tx sender paid for the gasUsed * (tip + block baseFee).
	actual = new(big.Int).Sub(funds, state.GetBalance(addr1).ToBig())
	expected = new(big.Int).SetUint64(block.GasUsed() * (block.Transactions()[0].GasTipCap().Uint64() + block.BaseFee().Uint64()))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender balance incorrect: expected %d, got %d", expected, actual)
	}

	blocks, _ = GenerateChain(gspec.Config, block, engine, genDb, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})

		txdata := &types.LegacyTx{
			Nonce:    0,
			To:       &aa,
			Gas:      30000,
			GasPrice: newGwei(5),
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key2)

		b.AddTx(tx)
	})

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block = chain.GetBlockByNumber(2)
	state, _ = chain.State()
	effectiveTip := block.Transactions()[0].GasTipCap().Uint64() - block.BaseFee().Uint64()

	// 6+5: Ensure that miner received only the tx's effective tip.
	actual = state.GetBalance(block.Coinbase()).ToBig()
	expected = new(big.Int).Add(
		new(big.Int).SetUint64(block.GasUsed()*effectiveTip),
		ethash.ConstantinopleBlockReward.ToBig(),
	)
	if actual.Cmp(expected) != 0 {
		t.Fatalf("miner balance incorrect: expected %d, got %d", expected, actual)
	}

	// 4: Ensure the tx sender paid for the gasUsed * (effectiveTip + block baseFee).
	actual = new(big.Int).Sub(funds, state.GetBalance(addr2).ToBig())
	expected = new(big.Int).SetUint64(block.GasUsed() * (effectiveTip + block.BaseFee().Uint64()))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender balance incorrect: expected %d, got %d", expected, actual)
	}
}

// Tests the scenario the chain is requested to another point with the missing state.
// It expects the state is recovered and all relevant chain markers are set correctly.
func TestSetCanonical(t *testing.T) {
	testSetCanonical(t, rawdb.HashScheme)
	testSetCanonical(t, rawdb.PathScheme)
}

func testSetCanonical(t *testing.T, scheme string) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	var (
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(100000000000000000)
		gspec   = &Genesis{
			Config:  params.TestChainConfig,
			Alloc:   types.GenesisAlloc{address: {Balance: funds}},
			BaseFee: big.NewInt(params.InitialBaseFee),
		}
		signer = types.LatestSigner(gspec.Config)
		engine = ethash.NewFaker()
	)
	// Generate and import the canonical chain
	_, canon, _ := GenerateChainWithGenesis(gspec, engine, 2*TriesInMemory, func(i int, gen *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(gen.TxNonce(address), common.Address{0x00}, big.NewInt(1000), params.TxGas, gen.header.BaseFee, nil), signer, key)
		if err != nil {
			panic(err)
		}
		gen.AddTx(tx)
	})
	diskdb, _ := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), t.TempDir(), "", false, false, false, false, false)
	defer diskdb.Close()

	chain, err := NewBlockChain(diskdb, DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()

	if n, err := chain.InsertChain(canon); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	// Generate the side chain and import them
	_, side, _ := GenerateChainWithGenesis(gspec, engine, 2*TriesInMemory, func(i int, gen *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(gen.TxNonce(address), common.Address{0x00}, big.NewInt(1), params.TxGas, gen.header.BaseFee, nil), signer, key)
		if err != nil {
			panic(err)
		}
		gen.AddTx(tx)
	})
	for _, block := range side {
		err := chain.InsertBlockWithoutSetHead(block)
		if err != nil {
			t.Fatalf("Failed to insert into chain: %v", err)
		}
	}
	for _, block := range side {
		got := chain.GetBlockByHash(block.Hash())
		if got == nil {
			t.Fatalf("Lost the inserted block")
		}
	}

	// Set the chain head to the side chain, ensure all the relevant markers are updated.
	verify := func(head *types.Block) {
		if chain.CurrentBlock().Hash() != head.Hash() {
			t.Fatalf("Unexpected block hash, want %x, got %x", head.Hash(), chain.CurrentBlock().Hash())
		}
		if chain.CurrentSnapBlock().Hash() != head.Hash() {
			t.Fatalf("Unexpected fast block hash, want %x, got %x", head.Hash(), chain.CurrentSnapBlock().Hash())
		}
		if chain.CurrentHeader().Hash() != head.Hash() {
			t.Fatalf("Unexpected head header, want %x, got %x", head.Hash(), chain.CurrentHeader().Hash())
		}
		if !chain.HasState(head.Root()) {
			t.Fatalf("Lost block state %v %x", head.Number(), head.Hash())
		}
	}
	chain.SetCanonical(side[len(side)-1])
	verify(side[len(side)-1])

	// Reset the chain head to original chain
	chain.SetCanonical(canon[TriesInMemory-1])
	verify(canon[TriesInMemory-1])
}

// TestCanonicalHashMarker tests all the canonical hash markers are updated/deleted
// correctly in case reorg is called.
func TestCanonicalHashMarker(t *testing.T) {
	testCanonicalHashMarker(t, rawdb.HashScheme)
	testCanonicalHashMarker(t, rawdb.PathScheme)
}

func testCanonicalHashMarker(t *testing.T, scheme string) {
	var cases = []struct {
		forkA int
		forkB int
	}{
		// ForkA: 10 blocks
		// ForkB: 1 blocks
		//
		// reorged:
		//      markers [2, 10] should be deleted
		//      markers [1] should be updated
		{10, 1},

		// ForkA: 10 blocks
		// ForkB: 2 blocks
		//
		// reorged:
		//      markers [3, 10] should be deleted
		//      markers [1, 2] should be updated
		{10, 2},

		// ForkA: 10 blocks
		// ForkB: 10 blocks
		//
		// reorged:
		//      markers [1, 10] should be updated
		{10, 10},

		// ForkA: 10 blocks
		// ForkB: 11 blocks
		//
		// reorged:
		//      markers [1, 11] should be updated
		{10, 11},
	}
	for _, c := range cases {
		var (
			gspec = &Genesis{
				Config:  params.TestChainConfig,
				Alloc:   types.GenesisAlloc{},
				BaseFee: big.NewInt(params.InitialBaseFee),
			}
			engine = ethash.NewFaker()
		)
		_, forkA, _ := GenerateChainWithGenesis(gspec, engine, c.forkA, func(i int, gen *BlockGen) {})
		_, forkB, _ := GenerateChainWithGenesis(gspec, engine, c.forkB, func(i int, gen *BlockGen) {})

		// Initialize test chain
		chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), DefaultCacheConfigWithScheme(scheme), gspec, nil, engine, vm.Config{}, nil, nil)
		if err != nil {
			t.Fatalf("failed to create tester chain: %v", err)
		}
		// Insert forkA and forkB, the canonical should on forkA still
		if n, err := chain.InsertChain(forkA); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", n, err)
		}
		if n, err := chain.InsertChain(forkB); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", n, err)
		}

		verify := func(head *types.Block) {
			if chain.CurrentBlock().Hash() != head.Hash() {
				t.Fatalf("Unexpected block hash, want %x, got %x", head.Hash(), chain.CurrentBlock().Hash())
			}
			if chain.CurrentSnapBlock().Hash() != head.Hash() {
				t.Fatalf("Unexpected fast block hash, want %x, got %x", head.Hash(), chain.CurrentSnapBlock().Hash())
			}
			if chain.CurrentHeader().Hash() != head.Hash() {
				t.Fatalf("Unexpected head header, want %x, got %x", head.Hash(), chain.CurrentHeader().Hash())
			}
			if !chain.HasState(head.Root()) {
				t.Fatalf("Lost block state %v %x", head.Number(), head.Hash())
			}
		}

		// Switch canonical chain to forkB if necessary
		if len(forkA) < len(forkB) {
			verify(forkB[len(forkB)-1])
		} else {
			verify(forkA[len(forkA)-1])
			chain.SetCanonical(forkB[len(forkB)-1])
			verify(forkB[len(forkB)-1])
		}

		// Ensure all hash markers are updated correctly
		for i := 0; i < len(forkB); i++ {
			block := forkB[i]
			hash := chain.GetCanonicalHash(block.NumberU64())
			if hash != block.Hash() {
				t.Fatalf("Unexpected canonical hash %d", block.NumberU64())
			}
		}
		if c.forkA > c.forkB {
			for i := uint64(c.forkB) + 1; i <= uint64(c.forkA); i++ {
				hash := chain.GetCanonicalHash(i)
				if hash != (common.Hash{}) {
					t.Fatalf("Unexpected canonical hash %d", i)
				}
			}
		}
		chain.Stop()
	}
}

func TestCreateThenDeletePreByzantium(t *testing.T) {
	// We use Ropsten chain config instead of Testchain config, this is
	// deliberate: we want to use pre-byz rules where we have intermediate state roots
	// between transactions.
	testCreateThenDelete(t, &params.ChainConfig{
		ChainID:        big.NewInt(3),
		HomesteadBlock: big.NewInt(0),
		EIP150Block:    big.NewInt(0),
		EIP155Block:    big.NewInt(10),
		EIP158Block:    big.NewInt(10),
		ByzantiumBlock: big.NewInt(1_700_000),
	})
}
func TestCreateThenDeletePostByzantium(t *testing.T) {
	testCreateThenDelete(t, params.TestChainConfig)
}

// testCreateThenDelete tests a creation and subsequent deletion of a contract, happening
// within the same block.
func testCreateThenDelete(t *testing.T, config *params.ChainConfig) {
	var (
		engine = ethash.NewFaker()
		// A sender who makes transactions, has some funds
		key, _      = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address     = crypto.PubkeyToAddress(key.PublicKey)
		destAddress = crypto.CreateAddress(address, 0)
		funds       = big.NewInt(1000000000000000)
	)

	// runtime code is 	0x60ffff : PUSH1 0xFF SELFDESTRUCT, a.k.a SELFDESTRUCT(0xFF)
	code := append([]byte{0x60, 0xff, 0xff}, make([]byte, 32-3)...)
	initCode := []byte{
		// SSTORE 1:1
		byte(vm.PUSH1), 0x1,
		byte(vm.PUSH1), 0x1,
		byte(vm.SSTORE),
		// Get the runtime-code on the stack
		byte(vm.PUSH32)}
	initCode = append(initCode, code...)
	initCode = append(initCode, []byte{
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x3, // size
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.RETURN), // return 3 bytes of zero-code
	}...)
	gspec := &Genesis{
		Config: config,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
		},
	}
	nonce := uint64(0)
	signer := types.HomesteadSigner{}
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 2, func(i int, b *BlockGen) {
		fee := big.NewInt(1)
		if b.header.BaseFee != nil {
			fee = b.header.BaseFee
		}
		b.SetCoinbase(common.Address{1})
		tx, _ := types.SignNewTx(key, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      100000,
			Data:     initCode,
		})
		nonce++
		b.AddTx(tx)
		tx, _ = types.SignNewTx(key, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      100000,
			To:       &destAddress,
		})
		b.AddTx(tx)
		nonce++
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{
		//Debug:  true,
		//Tracer: logger.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()
	// Import the blocks
	for _, block := range blocks {
		if _, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
	}
}

func TestDeleteThenCreate(t *testing.T) {
	var (
		engine      = ethash.NewFaker()
		key, _      = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address     = crypto.PubkeyToAddress(key.PublicKey)
		factoryAddr = crypto.CreateAddress(address, 0)
		funds       = big.NewInt(1000000000000000)
	)
	/*
		contract Factory {
		  function deploy(bytes memory code) public {
			address addr;
			assembly {
			  addr := create2(0, add(code, 0x20), mload(code), 0)
			  if iszero(extcodesize(addr)) {
				revert(0, 0)
			  }
			}
		  }
		}
	*/
	factoryBIN := common.Hex2Bytes("608060405234801561001057600080fd5b50610241806100206000396000f3fe608060405234801561001057600080fd5b506004361061002a5760003560e01c80627743601461002f575b600080fd5b610049600480360381019061004491906100d8565b61004b565b005b6000808251602084016000f59050803b61006457600080fd5b5050565b600061007b61007684610146565b610121565b905082815260208101848484011115610097576100966101eb565b5b6100a2848285610177565b509392505050565b600082601f8301126100bf576100be6101e6565b5b81356100cf848260208601610068565b91505092915050565b6000602082840312156100ee576100ed6101f5565b5b600082013567ffffffffffffffff81111561010c5761010b6101f0565b5b610118848285016100aa565b91505092915050565b600061012b61013c565b90506101378282610186565b919050565b6000604051905090565b600067ffffffffffffffff821115610161576101606101b7565b5b61016a826101fa565b9050602081019050919050565b82818337600083830152505050565b61018f826101fa565b810181811067ffffffffffffffff821117156101ae576101ad6101b7565b5b80604052505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b600080fd5b600080fd5b600080fd5b600080fd5b6000601f19601f830116905091905056fea2646970667358221220ea8b35ed310d03b6b3deef166941140b4d9e90ea2c92f6b41eb441daf49a59c364736f6c63430008070033")

	/*
		contract C {
			uint256 value;
			constructor() {
				value = 100;
			}
			function destruct() public payable {
				selfdestruct(payable(msg.sender));
			}
			receive() payable external {}
		}
	*/
	contractABI := common.Hex2Bytes("6080604052348015600f57600080fd5b5060646000819055506081806100266000396000f3fe608060405260043610601f5760003560e01c80632b68b9c614602a576025565b36602557005b600080fd5b60306032565b005b3373ffffffffffffffffffffffffffffffffffffffff16fffea2646970667358221220ab749f5ed1fcb87bda03a74d476af3f074bba24d57cb5a355e8162062ad9a4e664736f6c63430008070033")
	contractAddr := crypto.CreateAddress2(factoryAddr, [32]byte{}, crypto.Keccak256(contractABI))

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
		},
	}
	nonce := uint64(0)
	signer := types.HomesteadSigner{}
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 2, func(i int, b *BlockGen) {
		fee := big.NewInt(1)
		if b.header.BaseFee != nil {
			fee = b.header.BaseFee
		}
		b.SetCoinbase(common.Address{1})

		// Block 1
		if i == 0 {
			tx, _ := types.SignNewTx(key, signer, &types.LegacyTx{
				Nonce:    nonce,
				GasPrice: new(big.Int).Set(fee),
				Gas:      500000,
				Data:     factoryBIN,
			})
			nonce++
			b.AddTx(tx)

			data := common.Hex2Bytes("00774360000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000a76080604052348015600f57600080fd5b5060646000819055506081806100266000396000f3fe608060405260043610601f5760003560e01c80632b68b9c614602a576025565b36602557005b600080fd5b60306032565b005b3373ffffffffffffffffffffffffffffffffffffffff16fffea2646970667358221220ab749f5ed1fcb87bda03a74d476af3f074bba24d57cb5a355e8162062ad9a4e664736f6c6343000807003300000000000000000000000000000000000000000000000000")
			tx, _ = types.SignNewTx(key, signer, &types.LegacyTx{
				Nonce:    nonce,
				GasPrice: new(big.Int).Set(fee),
				Gas:      500000,
				To:       &factoryAddr,
				Data:     data,
			})
			b.AddTx(tx)
			nonce++
		} else {
			// Block 2
			tx, _ := types.SignNewTx(key, signer, &types.LegacyTx{
				Nonce:    nonce,
				GasPrice: new(big.Int).Set(fee),
				Gas:      500000,
				To:       &contractAddr,
				Data:     common.Hex2Bytes("2b68b9c6"), // destruct
			})
			nonce++
			b.AddTx(tx)

			data := common.Hex2Bytes("00774360000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000a76080604052348015600f57600080fd5b5060646000819055506081806100266000396000f3fe608060405260043610601f5760003560e01c80632b68b9c614602a576025565b36602557005b600080fd5b60306032565b005b3373ffffffffffffffffffffffffffffffffffffffff16fffea2646970667358221220ab749f5ed1fcb87bda03a74d476af3f074bba24d57cb5a355e8162062ad9a4e664736f6c6343000807003300000000000000000000000000000000000000000000000000")
			tx, _ = types.SignNewTx(key, signer, &types.LegacyTx{
				Nonce:    nonce,
				GasPrice: new(big.Int).Set(fee),
				Gas:      500000,
				To:       &factoryAddr, // re-creation
				Data:     data,
			})
			b.AddTx(tx)
			nonce++
		}
	})
	// Import the canonical chain
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	for _, block := range blocks {
		if _, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
	}
}

// TestTransientStorageReset ensures the transient storage is wiped correctly
// between transactions.
func TestTransientStorageReset(t *testing.T) {
	var (
		engine      = ethash.NewFaker()
		key, _      = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address     = crypto.PubkeyToAddress(key.PublicKey)
		destAddress = crypto.CreateAddress(address, 0)
		funds       = big.NewInt(1000000000000000)
		vmConfig    = vm.Config{
			ExtraEips: []int{1153}, // Enable transient storage EIP
		}
	)
	code := append([]byte{
		// TLoad value with location 1
		byte(vm.PUSH1), 0x1,
		byte(vm.TLOAD),

		// PUSH location
		byte(vm.PUSH1), 0x1,

		// SStore location:value
		byte(vm.SSTORE),
	}, make([]byte, 32-6)...)
	initCode := []byte{
		// TSTORE 1:1
		byte(vm.PUSH1), 0x1,
		byte(vm.PUSH1), 0x1,
		byte(vm.TSTORE),

		// Get the runtime-code on the stack
		byte(vm.PUSH32)}
	initCode = append(initCode, code...)
	initCode = append(initCode, []byte{
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x6, // size
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.RETURN), // return 6 bytes of zero-code
	}...)
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address: {Balance: funds},
		},
	}
	nonce := uint64(0)
	signer := types.HomesteadSigner{}
	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		fee := big.NewInt(1)
		if b.header.BaseFee != nil {
			fee = b.header.BaseFee
		}
		b.SetCoinbase(common.Address{1})
		tx, _ := types.SignNewTx(key, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      100000,
			Data:     initCode,
		})
		nonce++
		b.AddTxWithVMConfig(tx, vmConfig)

		tx, _ = types.SignNewTx(key, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      100000,
			To:       &destAddress,
		})
		b.AddTxWithVMConfig(tx, vmConfig)
		nonce++
	})

	// Initialize the blockchain with 1153 enabled.
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vmConfig, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()
	// Import the blocks
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("failed to insert into chain: %v", err)
	}
	// Check the storage
	state, err := chain.StateAt(chain.CurrentHeader().Root)
	if err != nil {
		t.Fatalf("Failed to load state %v", err)
	}
	loc := common.BytesToHash([]byte{1})
	slot := state.GetState(destAddress, loc)
	if slot != (common.Hash{}) {
		t.Fatalf("Unexpected dirty storage slot")
	}
}

func TestEIP3651(t *testing.T) {
	var (
		aa     = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
		bb     = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		engine = beacon.NewFaker()

		// A sender who makes transactions, has some funds
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		funds   = new(big.Int).Mul(common.Big1, big.NewInt(params.Ether))
		config  = *params.AllEthashProtocolChanges
		gspec   = &Genesis{
			Config: &config,
			Alloc: types.GenesisAlloc{
				addr1: {Balance: funds},
				addr2: {Balance: funds},
				// The address 0xAAAA sloads 0x00 and 0x01
				aa: {
					Code: []byte{
						byte(vm.PC),
						byte(vm.PC),
						byte(vm.SLOAD),
						byte(vm.SLOAD),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
				// The address 0xBBBB calls 0xAAAA
				bb: {
					Code: []byte{
						byte(vm.PUSH1), 0, // out size
						byte(vm.DUP1),  // out offset
						byte(vm.DUP1),  // out insize
						byte(vm.DUP1),  // in offset
						byte(vm.PUSH2), // address
						byte(0xaa),
						byte(0xaa),
						byte(vm.GAS), // gas
						byte(vm.DELEGATECALL),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
			},
		}
	)

	gspec.Config.BerlinBlock = common.Big0
	gspec.Config.LondonBlock = common.Big0
	gspec.Config.TerminalTotalDifficulty = common.Big0
	gspec.Config.TerminalTotalDifficultyPassed = true
	gspec.Config.ShanghaiTime = u64(0)
	signer := types.LatestSigner(gspec.Config)

	_, blocks, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(aa)
		// One transaction to Coinbase
		txdata := &types.DynamicFeeTx{
			ChainID:    gspec.Config.ChainID,
			Nonce:      0,
			To:         &bb,
			Gas:        500000,
			GasFeeCap:  newGwei(5),
			GasTipCap:  big.NewInt(2),
			AccessList: nil,
			Data:       []byte{},
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key1)

		b.AddTx(tx)
	})
	chain, err := NewBlockChain(rawdb.NewMemoryDatabase(), nil, gspec, nil, engine, vm.Config{Tracer: logger.NewMarkdownLogger(&logger.Config{}, os.Stderr).Hooks()}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	defer chain.Stop()
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block := chain.GetBlockByNumber(1)

	// 1+2: Ensure EIP-1559 access lists are accounted for via gas usage.
	innerGas := vm.GasQuickStep*2 + params.ColdSloadCostEIP2929*2
	expectedGas := params.TxGas + 5*vm.GasFastestStep + vm.GasQuickStep + 100 + innerGas // 100 because 0xaaaa is in access list
	if block.GasUsed() != expectedGas {
		t.Fatalf("incorrect amount of gas spent: expected %d, got %d", expectedGas, block.GasUsed())
	}

	state, _ := chain.State()

	// 3: Ensure that miner received only the tx's tip.
	actual := state.GetBalance(block.Coinbase()).ToBig()
	expected := new(big.Int).SetUint64(block.GasUsed() * block.Transactions()[0].GasTipCap().Uint64())
	if actual.Cmp(expected) != 0 {
		t.Fatalf("miner balance incorrect: expected %d, got %d", expected, actual)
	}

	// 4: Ensure the tx sender paid for the gasUsed * (tip + block baseFee).
	actual = new(big.Int).Sub(funds, state.GetBalance(addr1).ToBig())
	expected = new(big.Int).SetUint64(block.GasUsed() * (block.Transactions()[0].GasTipCap().Uint64() + block.BaseFee().Uint64()))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender balance incorrect: expected %d, got %d", expected, actual)
	}
}

type mockSatoshi struct {
	consensus.Engine
}

func (c *mockSatoshi) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (c *mockSatoshi) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	return nil
}

func (c *mockSatoshi) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	return nil
}

func (c *mockSatoshi) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	abort := make(chan<- struct{})
	results := make(chan error, len(headers))
	for i := 0; i < len(headers); i++ {
		results <- nil
	}
	return abort, results
}

func (c *mockSatoshi) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, _ *[]*types.Transaction, uncles []*types.Header, withdrawals []*types.Withdrawal,
	_ *[]*types.Receipt, _ *[]*types.Transaction, _ *uint64, tracer *tracing.Hooks) (err error) {
	return
}

func (c *mockSatoshi) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt, withdrawals []*types.Withdrawal, tracer *tracing.Hooks) (*types.Block, []*types.Receipt, error) {
	// Finalize block
	c.Finalize(chain, header, state, &txs, uncles, nil, nil, nil, nil, tracer)

	// Assign the final state root to header.
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))

	// Header seems complete, assemble into a block and return
	return types.NewBlock(header, txs, uncles, receipts, trie.NewStackTrie(nil)), receipts, nil
}

func (c *mockSatoshi) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	return big.NewInt(1)
}

func (c *mockSatoshi) BeforeValidateTx(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, txs *[]*types.Transaction, uncles []*types.Header,
	receipts *[]*types.Receipt, systemTxs *[]*types.Transaction, usedGas *uint64, tracer *tracing.Hooks) (err error) {
	return nil
}

func TestSatoshiBlobFeeReward(t *testing.T) {
	// Have N headers in the freezer
	frdir := t.TempDir()
	db, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create database with ancient backend")
	}
	config := params.SatoshiTestChainConfig
	gspec := &Genesis{
		Config: config,
		Alloc:  types.GenesisAlloc{testAddr: {Balance: new(big.Int).SetUint64(10 * params.Ether)}},
	}
	engine := &mockSatoshi{}
	chain, _ := NewBlockChain(db, nil, gspec, nil, engine, vm.Config{}, nil, nil)
	signer := types.LatestSigner(config)

	_, bs, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, gen *BlockGen) {
		tx, _ := makeMockTx(config, signer, testKey, gen.TxNonce(testAddr), gen.BaseFee().Uint64(), eip4844.CalcBlobFee(gen.ExcessBlobGas()).Uint64(), false)
		gen.AddTxWithChain(chain, tx)
		tx, sidecar := makeMockTx(config, signer, testKey, gen.TxNonce(testAddr), gen.BaseFee().Uint64(), eip4844.CalcBlobFee(gen.ExcessBlobGas()).Uint64(), true)
		gen.AddTxWithChain(chain, tx)
		gen.AddBlobSidecar(&types.BlobSidecar{
			BlobTxSidecar: *sidecar,
			TxIndex:       1,
			TxHash:        tx.Hash(),
		})
	})
	if _, err := chain.InsertChain(bs); err != nil {
		panic(err)
	}

	stateDB, err := chain.State()
	if err != nil {
		panic(err)
	}
	expect := new(big.Int)
	for _, block := range bs {
		receipts := chain.GetReceiptsByHash(block.Hash())
		for _, receipt := range receipts {
			if receipt.BlobGasPrice != nil {
				blob := receipt.BlobGasPrice.Mul(receipt.BlobGasPrice, new(big.Int).SetUint64(receipt.BlobGasUsed))
				expect.Add(expect, blob)
			}
			plain := receipt.EffectiveGasPrice.Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(receipt.GasUsed))
			expect.Add(expect, plain)
		}
	}
	actual := stateDB.GetBalance(params.SystemAddress)
	require.Equal(t, expect.Uint64(), actual.Uint64())
}

func makeMockTx(config *params.ChainConfig, signer types.Signer, key *ecdsa.PrivateKey, nonce uint64, baseFee uint64, blobBaseFee uint64, isBlobTx bool) (*types.Transaction, *types.BlobTxSidecar) {
	if !isBlobTx {
		raw := &types.DynamicFeeTx{
			ChainID:   config.ChainID,
			Nonce:     nonce,
			GasTipCap: big.NewInt(10),
			GasFeeCap: new(big.Int).SetUint64(baseFee + 10),
			Gas:       params.TxGas,
			To:        &common.Address{0x00},
			Value:     big.NewInt(0),
		}
		tx, _ := types.SignTx(types.NewTx(raw), signer, key)
		return tx, nil
	}
	sidecar := &types.BlobTxSidecar{
		Blobs:       []kzg4844.Blob{emptyBlob, emptyBlob},
		Commitments: []kzg4844.Commitment{emptyBlobCommit, emptyBlobCommit},
		Proofs:      []kzg4844.Proof{emptyBlobProof, emptyBlobProof},
	}
	raw := &types.BlobTx{
		ChainID:    uint256.MustFromBig(config.ChainID),
		Nonce:      nonce,
		GasTipCap:  uint256.NewInt(10),
		GasFeeCap:  uint256.NewInt(baseFee + 10),
		Gas:        params.TxGas,
		To:         common.Address{0x00},
		Value:      uint256.NewInt(0),
		BlobFeeCap: uint256.NewInt(blobBaseFee),
		BlobHashes: sidecar.BlobHashes(),
	}
	tx, _ := types.SignTx(types.NewTx(raw), signer, key)
	return tx, sidecar
}

func getFeeMarketGenesisAlloc(maxRewards, maxEvents, maxGas uint64) (accountAddress common.Address, account types.Account) {
	return common.HexToAddress(systemcontracts.FeeMarketContract), types.Account{
		Balance: big.NewInt(0),
		Storage: map[common.Hash]common.Hash{
			common.HexToHash("0x0000000000000000000000000000000000000001"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000002710"),
			common.HexToHash("0x0000000000000000000000000000000000000002"): common.BigToHash(big.NewInt(int64(maxRewards))),
			common.HexToHash("0x0000000000000000000000000000000000000003"): common.BigToHash(big.NewInt(int64(maxEvents))),
			common.HexToHash("0x0000000000000000000000000000000000000004"): common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000002"),
			common.HexToHash("0x0000000000000000000000000000000000000005"): common.BigToHash(big.NewInt(int64(maxGas))),
		},
		Code: common.FromHex("0x608060405234801561001057600080fd5b506004361061020a5760003560e01c80636b3ab9551161012a578063b1171724116100bd578063c81b16621161008c578063e1c7392a11610071578063e1c7392a146103ff578063f050828714610446578063f9a2bbc71461045957600080fd5b8063c81b1662146103ed578063dc927faf146103f657600080fd5b8063b1171724146103c9578063b3d676f3146103d2578063b3ee5a51146103db578063b95493b3146103e457600080fd5b8063943599fd116100f9578063943599fd146103905780639dc092621461039a578063a78abc16146103a3578063aa82dce1146103c057600080fd5b80636b3ab95514610362578063783028a9146103755780637cb6b2751461037e578063918f86741461038757600080fd5b80633b768160116101a25780635d25e51d116101715780635d25e51d146103345780635ded1bd61461033d5780635f2a9f41146103465780636911c8a71461034f57600080fd5b80633b768160146102ed5780633f2c0f64146102f657806343756e5c1461030957806354d44bf71461031257600080fd5b806318412eeb116101de57806318412eeb146102a15780632131c68c146102b657806325ee13e2146102db5780632a5d69b2146102e457600080fd5b806298fa221461020f57806304e9e3a4146102535780630c370ed01461028157806314c1e1f714610298575b600080fd5b61022261021d366004612030565b610462565b6040805173ffffffffffffffffffffffffffffffffffffffff90931683529015156020830152015b60405180910390f35b61025c61100781565b60405173ffffffffffffffffffffffffffffffffffffffff909116815260200161024a565b61028a60025481565b60405190815260200161024a565b61025c61100481565b6102b46102af366004611ee2565b6104ba565b005b60005461025c90610100900473ffffffffffffffffffffffffffffffffffffffff1681565b61025c61100581565b61025c61101281565b61025c61101481565b6102b4610304366004612007565b6104c8565b61025c61100181565b6102b46127106001556005600281905560038190556004819055620f42409055565b61025c61101181565b61028a60035481565b61028a60055481565b6102b461035d366004611f14565b6104d2565b6102b4610370366004611ec1565b6104e2565b61025c61100881565b61028a60045481565b61028a60015481565b61025c6201000181565b61025c61100681565b6000546103b09060ff1681565b604051901515815260200161024a565b61025c61101081565b61025c61100981565b61025c61101381565b61025c61101581565b61025c61101681565b61025c61100281565b61025c61100381565b6102b4600080547fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff001660011790556005600281905560038190556004819055620f42409055565b6102b4610454366004611f85565b6104ee565b61025c61100081565b6006818154811061047257600080fd5b600091825260209091206003909102015473ffffffffffffffffffffffffffffffffffffffff8116915074010000000000000000000000000000000000000000900460ff1682565b6104c48282610500565b5050565b6104c48282610a82565b6104dd838383610b38565b505050565b6104eb81610fd7565b50565b6104fa848484846111ff565b50505050565b600061050b83611762565b90506000805b6006838154811061053257634e487b7160e01b600052603260045260246000fd5b90600052602060002090600302016001018054905081101561099d5760005b6006848154811061057257634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160010182815481106105a257634e487b7160e01b600052603260045260246000fd5b90600052602060002090600302016002018054905081101561098a578473ffffffffffffffffffffffffffffffffffffffff16600685815481106105f657634e487b7160e01b600052603260045260246000fd5b9060005260206000209060030201600101838154811061062657634e487b7160e01b600052603260045260246000fd5b9060005260206000209060030201600201828154811061065657634e487b7160e01b600052603260045260246000fd5b60009182526020909120015473ffffffffffffffffffffffffffffffffffffffff161415610978576006848154811061069f57634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160010182815481106106cf57634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160020160016006868154811061070357634e487b7160e01b600052603260045260246000fd5b9060005260206000209060030201600101848154811061073357634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160020180549050610752919061211f565b8154811061077057634e487b7160e01b600052603260045260246000fd5b906000526020600020016006858154811061079b57634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160010183815481106107cb57634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160020182815481106107fb57634e487b7160e01b600052603260045260246000fd5b6000918252602090912082549101805473ffffffffffffffffffffffffffffffffffffffff9092167fffffffffffffffffffffffff000000000000000000000000000000000000000083168117825592547fffffffffffffffffffff00000000000000000000000000000000000000000000909216909217740100000000000000000000000000000000000000009182900461ffff1690910217905560068054859081106108b957634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160010182815481106108e957634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160020180548061091757634e487b7160e01b600052603160045260246000fd5b60008281526020902081017fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff90810180547fffffffffffffffffffff000000000000000000000000000000000000000000001690550190556001925061098a565b8061098281612136565b915050610551565b508061099581612136565b915050610511565b50806109f2576040517f6ec3bc8500000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff841660048201526024015b60405180910390fd5b8373ffffffffffffffffffffffffffffffffffffffff167f57d4073099735e9cca8e792596cbbfe773a7d12a5efcd88c7a2cf3e0a8eba00560068481548110610a4b57634e487b7160e01b600052603260045260246000fd5b60009182526020822060016003909202010154604051610a749290918252602082015260400190565b60405180910390a250505050565b6000610a8d83611762565b90508160068281548110610ab157634e487b7160e01b600052603260045260246000fd5b906000526020600020906003020160000160146101000a81548160ff0219169083151502179055508273ffffffffffffffffffffffffffffffffffffffff167f57d4073099735e9cca8e792596cbbfe773a7d12a5efcd88c7a2cf3e0a8eba005600080604051610b2b929190918252602082015260400190565b60405180910390a2505050565b6000610b4384611762565b90506003548351118015610b575750825115155b15610b8e576040517f2eb6de7a00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b60045482511115610bcb576040517f2eb6de7a00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b60005b8351811015610c5157610c0b848281518110610bfa57634e487b7160e01b600052603260045260246000fd5b602002602001015160400151611838565b610c3f848281518110610c2e57634e487b7160e01b600052603260045260246000fd5b6020026020010151602001516118d3565b80610c4981612136565b915050610bce565b5060068181548110610c7357634e487b7160e01b600052603260045260246000fd5b90600052602060002090600302016001016000610c90919061191d565b60005b8351811015610f8657600060068381548110610cbf57634e487b7160e01b600052603260045260246000fd5b90600052602060002090600302016001016001816001815401808255809150500390600052602060002090600302019050848281518110610d1057634e487b7160e01b600052603260045260246000fd5b60209081029190910101515181558451859083908110610d4057634e487b7160e01b600052603260045260246000fd5b6020026020010151602001518160010160006101000a81548163ffffffff021916908363ffffffff16021790555060005b858381518110610d9157634e487b7160e01b600052603260045260246000fd5b60200260200101516040015151811015610f7157600254868481518110610dc857634e487b7160e01b600052603260045260246000fd5b602002602001015160400151511115610e0d576040517fede0c82900000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b600282018054600181018255600091825260209091208751910190879085908110610e4857634e487b7160e01b600052603260045260246000fd5b6020026020010151604001518281518110610e7357634e487b7160e01b600052603260045260246000fd5b60209081029190910101515181547fffffffffffffffffffffffff00000000000000000000000000000000000000001673ffffffffffffffffffffffffffffffffffffffff9091161781558651879085908110610ee057634e487b7160e01b600052603260045260246000fd5b6020026020010151604001518281518110610f0b57634e487b7160e01b600052603260045260246000fd5b6020908102919091018101510151815461ffff90911674010000000000000000000000000000000000000000027fffffffffffffffffffff0000ffffffffffffffffffffffffffffffffffffffff90911617905580610f6981612136565b915050610d71565b50508080610f7e90612136565b915050610c93565b508251604080519182526000602083015273ffffffffffffffffffffffffffffffffffffffff8616917f57d4073099735e9cca8e792596cbbfe773a7d12a5efcd88c7a2cf3e0a8eba0059101610a74565b6000610fe282611762565b6006805491925090610ff69060019061211f565b8154811061101457634e487b7160e01b600052603260045260246000fd5b90600052602060002090600302016006828154811061104357634e487b7160e01b600052603260045260246000fd5b600091825260209091208254600390920201805473ffffffffffffffffffffffffffffffffffffffff9092167fffffffffffffffffffffffff000000000000000000000000000000000000000083168117825583547fffffffffffffffffffffff00000000000000000000000000000000000000000090931617740100000000000000000000000000000000000000009283900460ff161515909202919091178155600180830180546110f9928401919061193e565b506002828101805461110e92840191906119ea565b50905050600680548061113157634e487b7160e01b600052603160045260246000fd5b60008281526020812060037fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff9093019283020180547fffffffffffffffffffffff00000000000000000000000000000000000000000016815590611198600183018261191d565b6111a6600283016000611a92565b50509055604080516000808252602082015273ffffffffffffffffffffffffffffffffffffffff8416917f57d4073099735e9cca8e792596cbbfe773a7d12a5efcd88c7a2cf3e0a8eba005910160405180910390a25050565b60005b6006548110156112cd578473ffffffffffffffffffffffffffffffffffffffff166006828154811061124457634e487b7160e01b600052603260045260246000fd5b600091825260209091206003909102015473ffffffffffffffffffffffffffffffffffffffff1614156112bb576040517fdb36a8f000000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff861660048201526024016109e9565b806112c581612136565b915050611202565b5060035483511180156112e05750825115155b15611317576040517f2eb6de7a00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b60045482511115611354576040517f2eb6de7a00000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b60005b83518110156113b857611383848281518110610bfa57634e487b7160e01b600052603260045260246000fd5b6113a6848281518110610c2e57634e487b7160e01b600052603260045260246000fd5b806113b081612136565b915050611357565b506006805460018101825560009182526003027ff652222313e28459528d920b65115c16c04f3efc82aaedc97be59f3f377c0d3f01805483151574010000000000000000000000000000000000000000027fffffffffffffffffffffff00000000000000000000000000000000000000000090911673ffffffffffffffffffffffffffffffffffffffff881617178155905b84518110156117075760018083018054918201815560009081526020902086516003909202019086908390811061149157634e487b7160e01b600052603260045260246000fd5b602090810291909101015151815585518690839081106114c157634e487b7160e01b600052603260045260246000fd5b6020026020010151602001518160010160006101000a81548163ffffffff021916908363ffffffff16021790555060005b86838151811061151257634e487b7160e01b600052603260045260246000fd5b602002602001015160400151518110156116f25760025487848151811061154957634e487b7160e01b600052603260045260246000fd5b60200260200101516040015151111561158e576040517fede0c82900000000000000000000000000000000000000000000000000000000815260040160405180910390fd5b6002820180546001810182556000918252602090912088519101908890859081106115c957634e487b7160e01b600052603260045260246000fd5b60200260200101516040015182815181106115f457634e487b7160e01b600052603260045260246000fd5b60209081029190910101515181547fffffffffffffffffffffffff00000000000000000000000000000000000000001673ffffffffffffffffffffffffffffffffffffffff909116178155875188908590811061166157634e487b7160e01b600052603260045260246000fd5b602002602001015160400151828151811061168c57634e487b7160e01b600052603260045260246000fd5b6020908102919091018101510151815461ffff90911674010000000000000000000000000000000000000000027fffffffffffffffffffff0000ffffffffffffffffffffffffffffffffffffffff909116179055806116ea81612136565b9150506114f2565b505080806116ff90612136565b91505061144a565b508351604080519182526000602083015273ffffffffffffffffffffffffffffffffffffffff8716917f57d4073099735e9cca8e792596cbbfe773a7d12a5efcd88c7a2cf3e0a8eba005910160405180910390a25050505050565b6000805b6006548110156117ed578273ffffffffffffffffffffffffffffffffffffffff16600682815481106117a857634e487b7160e01b600052603260045260246000fd5b600091825260209091206003909102015473ffffffffffffffffffffffffffffffffffffffff1614156117db5792915050565b806117e581612136565b915050611766565b506040517f70de323100000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff831660048201526024016109e9565b6000805b82518110156118945782818151811061186557634e487b7160e01b600052603260045260246000fd5b60200260200101516020015161ffff16826118809190612107565b91508061188c81612136565b91505061183c565b5060015481146104c4576040517f1c8868d0000000000000000000000000000000000000000000000000000000008152600481018290526024016109e9565b6005548163ffffffff1611156104eb576040517f191aa42a00000000000000000000000000000000000000000000000000000000815263ffffffff821660048201526024016109e9565b50805460008255600302906000526020600020908101906104eb9190611ab3565b8280548282559060005260206000209060030281019282156119da5760005260206000209160030282015b828111156119da578254825560018084015490830180547fffffffffffffffffffffffffffffffffffffffffffffffffffffffff000000001663ffffffff90921691909117905560028084018054859285926119c89291840191611b00565b50505091600301919060030190611969565b506119e6929150611ab3565b5090565b828054828255906000526020600020906003028101928215611a865760005260206000209160030282015b82811115611a86578254825560018084015490830180547fffffffffffffffffffffffffffffffffffffffffffffffffffffffff000000001663ffffffff9092169190911790556002808401805485928592611a749291840191611b00565b50505091600301919060030190611a15565b506119e6929150611bd6565b50805460008255600302906000526020600020908101906104eb9190611bd6565b808211156119e65760008082556001820180547fffffffffffffffffffffffffffffffffffffffffffffffffffffffff00000000169055611af76002830182611c23565b50600301611ab3565b828054828255906000526020600020908101928215611bca5760005260206000209182015b82811115611bca578254825473ffffffffffffffffffffffffffffffffffffffff9091167fffffffffffffffffffffffff000000000000000000000000000000000000000082168117845584547fffffffffffffffffffff0000000000000000000000000000000000000000000090921617740100000000000000000000000000000000000000009182900461ffff1690910217825560019283019290910190611b25565b506119e6929150611c3d565b808211156119e65760008082556001820180547fffffffffffffffffffffffffffffffffffffffffffffffffffffffff00000000169055611c1a6002830182611c23565b50600301611bd6565b50805460008255906000526020600020908101906104eb91905b5b808211156119e65780547fffffffffffffffffffff00000000000000000000000000000000000000000000168155600101611c3e565b803573ffffffffffffffffffffffffffffffffffffffff81168114611c9857600080fd5b919050565b600082601f830112611cad578081fd5b81356020611cc2611cbd836120e3565b612094565b80838252828201915082860187848660051b8901011115611ce1578586fd5b855b85811015611d2257813567ffffffffffffffff811115611d01578788fd5b611d0f8a87838c0101611dbf565b8552509284019290840190600101611ce3565b5090979650505050505050565b600082601f830112611d3f578081fd5b81356020611d4f611cbd836120e3565b80838252828201915082860187848660051b8901011115611d6e578586fd5b855b85811015611d2257813567ffffffffffffffff811115611d8e578788fd5b611d9c8a87838c0101611dbf565b8552509284019290840190600101611d70565b80358015158114611c9857600080fd5b600060608284031215611dd0578081fd5b611dd8612048565b90508135815260208083013563ffffffff81168114611df657600080fd5b8282015260408381013567ffffffffffffffff811115611e1557600080fd5b8401601f81018613611e2657600080fd5b8035611e34611cbd826120e3565b80828252858201915085840189878560061b8701011115611e5457600080fd5b600094505b83851015611eaf5785818b031215611e7057600080fd5b611e78612071565b611e8182611c74565b81528782013561ffff81168114611e9757600080fd5b81890152835260019490940193918601918501611e59565b50808588015250505050505092915050565b600060208284031215611ed2578081fd5b611edb82611c74565b9392505050565b60008060408385031215611ef4578081fd5b611efd83611c74565b9150611f0b60208401611c74565b90509250929050565b600080600060608486031215611f28578081fd5b611f3184611c74565b9250602084013567ffffffffffffffff80821115611f4d578283fd5b611f5987838801611c9d565b93506040860135915080821115611f6e578283fd5b50611f7b86828701611d2f565b9150509250925092565b60008060008060808587031215611f9a578081fd5b611fa385611c74565b9350602085013567ffffffffffffffff80821115611fbf578283fd5b611fcb88838901611c9d565b94506040870135915080821115611fe0578283fd5b50611fed87828801611d2f565b925050611ffc60608601611daf565b905092959194509250565b60008060408385031215612019578182fd5b61202283611c74565b9150611f0b60208401611daf565b600060208284031215612041578081fd5b5035919050565b6040516060810167ffffffffffffffff8111828210171561206b5761206b612185565b60405290565b6040805190810167ffffffffffffffff8111828210171561206b5761206b612185565b604051601f82017fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe016810167ffffffffffffffff811182821017156120db576120db612185565b604052919050565b600067ffffffffffffffff8211156120fd576120fd612185565b5060051b60200190565b6000821982111561211a5761211a61216f565b500190565b6000828210156121315761213161216f565b500390565b60007fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff8214156121685761216861216f565b5060010190565b634e487b7160e01b600052601160045260246000fd5b634e487b7160e01b600052604160045260246000fdfea2646970667358221220c92b3ced902c2a91f2df93027aedb46b63ec392da1a3fb54abbee218c31a08ad64736f6c63430008040033"),
	}
}

func TestSatoshiFeeMarket(t *testing.T) {
	config := params.SatoshiTestChainConfig
	gspec := &Genesis{
		Config: config,
		Alloc: types.GenesisAlloc{
			testAddr: {Balance: new(big.Int).SetUint64(10 * params.Ether)},
		},
	}

	feeMarketAddress, feeMarketAccount := getFeeMarketGenesisAlloc(2, 2, 1000000)
	gspec.Alloc[feeMarketAddress] = feeMarketAccount

	rewardRecipient := common.HexToAddress("0x8f10d3a6283672ecfaeea0377d460bded489ec44")

	/*
		contract Counter {
			uint256 public number;

			event Increment(uint256 number);

			function init() external {
					number = 10;
			}

			function setNumber(uint256 newNumber) public {
					number = newNumber;
					emit Increment(number);
					// revert();
			}
			function increment() public {
					number++;

					emit Increment(number);
			}
		}
	*/
	counterBIN := common.Hex2Bytes("608060405234801561001057600080fd5b50610194806100206000396000f3fe608060405234801561001057600080fd5b506004361061004c5760003560e01c80633fb5c1cb146100515780638381f58a14610066578063d09de08a14610081578063e1c7392a14610089575b600080fd5b61006461005f36600461011f565b610093565b005b61006f60005481565b60405190815260200160405180910390f35b6100646100ce565b610064600a600055565b60008190556040518181527f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a819060200160405180910390a150565b6000805490806100dd83610137565b91905055507f51af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8160005460405161011591815260200190565b60405180910390a1565b600060208284031215610130578081fd5b5035919050565b600060001982141561015757634e487b7160e01b81526011600452602481fd5b506001019056fea26469706673582212203377405d46b8a079d1af97ea43d94b569eebc0dadc089dd96f37ce59cfe4c92c64736f6c63430008040033")

	// Initialize blockchain
	frdir := t.TempDir()
	db, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false, false, false, false, false)
	if err != nil {
		t.Fatalf("failed to create database with ancient backend")
	}
	engine := &mockSatoshi{}
	chain, _ := NewBlockChain(db, nil, gspec, nil, engine, vm.Config{}, nil, nil)
	signer := types.LatestSigner(config)

	var counterContractAddress common.Address
	nonce := uint64(0)
	_, bs, _ := GenerateChainWithGenesis(gspec, engine, 1, func(i int, b *BlockGen) {
		fee := big.NewInt(1)
		b.SetCoinbase(common.Address{1})

		// Deploy counter contract
		tx, _ := types.SignNewTx(testKey, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      500000,
			Data:     counterBIN,
		})
		counterContractAddress = crypto.CreateAddress(testAddr, nonce)
		nonce++
		b.AddTxWithChain(chain, tx)

		// Add configuration for the deployed contract
		configurationAddConfig := "f05082870000000000000000000000003A220f351252089D385b29beca14e27F204c296A000000000000000000000000000000000000000000000000000000000000008000000000000000000000000000000000000000000000000000000000000002e0000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000014051af157c2eee40f68107a47a49c32fbbeb0a3c9e5cd37aa56e88e6be92368a8100000000000000000000000000000000000000000000000000000000000186a0000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000020000000000000000000000008f10d3a6283672ecfaeea0377d460bded489ec440000000000000000000000000000000000000000000000000000000000002328000000000000000000000000000000000000000000000000000000000000078900000000000000000000000000000000000000000000000000000000000003e80335b51418df6ad87c7638414b2dd16910635533ebf9090fab3f0fdd07a515080000000000000000000000000000000000000000000000000000000000030d40000000000000000000000000000000000000000000000000000000000000006000000000000000000000000000000000000000000000000000000000000000020000000000000000000000008f10d3a6283672ecfaeea0377d460bded489ec440000000000000000000000000000000000000000000000000000000000002328000000000000000000000000000000000000000000000000000000000000078900000000000000000000000000000000000000000000000000000000000003e80000000000000000000000000000000000000000000000000000000000000000"

		// Replace the counterContractAddress in add config call data
		configurationAddConfig = strings.ReplaceAll(configurationAddConfig, "3A220f351252089D385b29beca14e27F204c296A", counterContractAddress.Hex()[2:])

		// Add custom rewards address
		configurationAddConfig = strings.ReplaceAll(configurationAddConfig, "8f10d3a6283672ecfaeea0377d460bded489ec44", rewardRecipient.Hex()[2:])

		tx, _ = types.SignNewTx(testKey, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      500000,
			To:       &feeMarketAddress,
			Data:     common.Hex2Bytes(configurationAddConfig),
		})
		b.AddTxWithChain(chain, tx)
		nonce++

		// Call contract
		data := common.Hex2Bytes("3fb5c1cb000000000000000000000000000000000000000000000000000000000000000a")
		tx, _ = types.SignNewTx(testKey, signer, &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: new(big.Int).Set(fee),
			Gas:      500000,
			To:       &counterContractAddress,
			Data:     data,
		})
		b.AddTxWithChain(chain, tx)
		nonce++
	})

	if _, err := chain.InsertChain(bs); err != nil {
		panic(err)
	}

	stateDB, err := chain.State()
	if err != nil {
		panic(err)
	}

	for _, block := range bs {
		txGasUsed := uint64(0)
		receipts := chain.GetReceiptsByHash(block.Hash())
		for _, receipt := range receipts {
			txGasUsed += receipt.GasUsed
		}

		fmt.Println("txGasUsed:", txGasUsed, block.GasUsed())
	}

	// Check balance of configured reward recipient
	expect := big.NewInt(90000)
	actual := stateDB.GetBalance(rewardRecipient)
	require.Equal(t, expect.Uint64(), actual.Uint64())
	t.Fail()
}
