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
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
)

type BlockValidatorOption func(*BlockValidator) *BlockValidator

func EnableRemoteVerifyManager(remoteValidator *remoteVerifyManager) BlockValidatorOption {
	return func(bv *BlockValidator) *BlockValidator {
		bv.remoteValidator = remoteValidator
		return bv
	}
}

// BlockValidator is responsible for validating block headers, uncles and
// processed state.
//
// BlockValidator implements Validator.
type BlockValidator struct {
	config          *params.ChainConfig // Chain configuration options
	bc              *BlockChain         // Canonical block chain
	engine          consensus.Engine    // Consensus engine used for validating
	remoteValidator *remoteVerifyManager
}

// NewBlockValidator returns a new block validator which is safe for re-use
func NewBlockValidator(config *params.ChainConfig, blockchain *BlockChain, engine consensus.Engine, opts ...BlockValidatorOption) *BlockValidator {
	validator := &BlockValidator{
		config: config,
		engine: engine,
		bc:     blockchain,
	}

	for _, opt := range opts {
		validator = opt(validator)
	}

	return validator
}

// ValidateListsInBody validates that UncleHash, WithdrawalsHash, and WithdrawalsHash correspond to the lists in the block body, respectively.
func ValidateListsInBody(block *types.Block) error {
	header := block.Header()
	if hash := types.CalcUncleHash(block.Uncles()); hash != header.UncleHash {
		return fmt.Errorf("uncle root hash mismatch (header value %x, calculated %x)", header.UncleHash, hash)
	}
	if hash := types.DeriveSha(block.Transactions(), trie.NewStackTrie(nil)); hash != header.TxHash {
		return fmt.Errorf("transaction root hash mismatch: have %x, want %x", hash, header.TxHash)
	}
	// Withdrawals are present after the Shanghai fork.
	if header.WithdrawalsHash != nil {
		// Withdrawals list must be present in body after Shanghai.
		if block.Withdrawals() == nil {
			return errors.New("missing withdrawals in block body")
		}
		if hash := types.DeriveSha(block.Withdrawals(), trie.NewStackTrie(nil)); hash != *header.WithdrawalsHash {
			return fmt.Errorf("withdrawals root hash mismatch (header value %x, calculated %x)", *header.WithdrawalsHash, hash)
		}
	} else if block.Withdrawals() != nil { // Withdrawals turn into empty from nil when BlockBody has Sidecars
		// Withdrawals are not allowed prior to shanghai fork
		return errors.New("withdrawals present in block body")
	}
	return nil
}

// ValidateBody validates the given block's uncles and verifies the block
// header's transaction and uncle roots. The headers are assumed to be already
// validated at this point.
func (v *BlockValidator) ValidateBody(block *types.Block) error {
	// Check whether the block is already imported.
	if v.bc.HasBlockAndState(block.Hash(), block.NumberU64()) {
		return ErrKnownBlock
	}
	// Header validity is known at this point. Here we verify that uncles, transactions
	// and withdrawals given in the block body match the header.
	header := block.Header()
	if err := v.engine.VerifyUncles(v.bc, block); err != nil {
		return err
	}

	validateFuns := []func() error{
		func() error {
			return ValidateListsInBody(block)
		},
		func() error {
			// Blob transactions may be present after the Cancun fork.
			var blobs int
			for i, tx := range block.Transactions() {
				// Count the number of blobs to validate against the header's blobGasUsed
				blobs += len(tx.BlobHashes())

				// If the tx is a blob tx, it must NOT have a sidecar attached to be valid in a block.
				if tx.BlobTxSidecar() != nil {
					return fmt.Errorf("unexpected blob sidecar in transaction at index %d", i)
				}

				// The individual checks for blob validity (version-check + not empty)
				// happens in StateTransition.
			}

			// Check blob gas usage.
			if header.BlobGasUsed != nil {
				if want := *header.BlobGasUsed / params.BlobTxBlobGasPerBlob; uint64(blobs) != want { // div because the header is surely good vs the body might be bloated
					return fmt.Errorf("blob gas used mismatch (header %v, calculated %v)", *header.BlobGasUsed, blobs*params.BlobTxBlobGasPerBlob)
				}
			} else {
				if blobs > 0 {
					return errors.New("data blobs present in block body")
				}
			}
			return nil
		},
		func() error {
			if !v.bc.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
				if !v.bc.HasBlock(block.ParentHash(), block.NumberU64()-1) {
					return consensus.ErrUnknownAncestor
				}
				return consensus.ErrPrunedAncestor
			}
			return nil
		},
		func() error {
			if v.remoteValidator != nil && !v.remoteValidator.AncestorVerified(block.Header()) {
				return fmt.Errorf("%w, number: %s, hash: %s", ErrAncestorHasNotBeenVerified, block.Number(), block.Hash())
			}
			return nil
		},
	}
	validateRes := make(chan error, len(validateFuns))
	for _, f := range validateFuns {
		tmpFunc := f
		go func() {
			validateRes <- tmpFunc()
		}()
	}
	for i := 0; i < len(validateFuns); i++ {
		r := <-validateRes
		if r != nil {
			return r
		}
	}
	return nil
}

// ValidateState validates the various changes that happen after a state transition,
// such as amount of used gas, the receipt roots and the state root itself.
func (v *BlockValidator) ValidateState(block *types.Block, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error {
	header := block.Header()
	if block.GasUsed() != usedGas {
		return fmt.Errorf("invalid gas used (remote: %d local: %d)", block.GasUsed(), usedGas)
	}
	// Validate the received block's bloom with the one derived from the generated receipts.
	// For valid blocks this should always validate to true.
	validateFuns := []func() error{
		func() error {
			rbloom := types.CreateBloom(receipts)
			if rbloom != header.Bloom {
				return fmt.Errorf("invalid bloom (remote: %x  local: %x)", header.Bloom, rbloom)
			}
			return nil
		},
		func() error {
			receiptSha := types.DeriveSha(receipts, trie.NewStackTrie(nil))
			if receiptSha != header.ReceiptHash {
				return fmt.Errorf("invalid receipt root hash (remote: %x local: %x)", header.ReceiptHash, receiptSha)
			}
			return nil
		},
	}
	validateFuns = append(validateFuns, func() error {
		if root := statedb.IntermediateRoot(v.config.IsEIP158(header.Number)); header.Root != root {
			return fmt.Errorf("invalid merkle root (remote: %x local: %x) dberr: %w", header.Root, root, statedb.Error())
		}
		return nil
	})
	validateRes := make(chan error, len(validateFuns))
	for _, f := range validateFuns {
		tmpFunc := f
		go func() {
			validateRes <- tmpFunc()
		}()
	}

	var err error
	for i := 0; i < len(validateFuns); i++ {
		r := <-validateRes
		if r != nil && err == nil {
			err = r
		}
	}
	return err
}

func (v *BlockValidator) RemoteVerifyManager() *remoteVerifyManager {
	return v.remoteValidator
}

// CalcGasLimit computes the gas limit of the next block after parent. It aims
// to keep the baseline gas close to the provided target, and increase it towards
// the target if the baseline gas is lower.
func CalcGasLimit(parentGasLimit, desiredLimit uint64) uint64 {
	delta := parentGasLimit/params.GasLimitBoundDivisor - 1
	limit := parentGasLimit
	if desiredLimit < params.MinGasLimit {
		desiredLimit = params.MinGasLimit
	}
	// If we're outside our allowed gas range, we try to hone towards them
	if limit < desiredLimit {
		limit = parentGasLimit + delta
		if limit > desiredLimit {
			limit = desiredLimit
		}
		return limit
	}
	if limit > desiredLimit {
		limit = parentGasLimit - delta
		if limit < desiredLimit {
			limit = desiredLimit
		}
	}
	return limit
}
