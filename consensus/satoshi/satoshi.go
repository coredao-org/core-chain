package satoshi

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/holiman/uint256"
	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/gopool"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/consensus/misc/eip4844"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/feemarket"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

const (
	inMemorySnapshots  = 256   // Number of recent snapshots to keep in memory
	inMemorySignatures = 4096  // Number of recent block signatures to keep in memory
	inMemoryHeaders    = 86400 // Number of recent headers to keep in memory for double sign detection,

	checkpointInterval   = 1024        // Number of blocks after which to save the snapshot to the database
	defaultEpochLength   = uint64(100) // Default number of blocks of checkpoint to update validatorSet from contract
	defaultRoundInterval = 86400       // Default number of seconds to turn round

	extraVanity      = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal        = 65 // Fixed number of extra-data suffix bytes reserved for signer seal
	nextForkHashSize = 4  // Fixed number of extra-data suffix bytes reserved for nextForkHash.

	validatorBytesLength = common.AddressLength
	wiggleTime           = uint64(1) // second, Random delay (per signer) to allow concurrent signers
	initialBackOffTime   = uint64(1) // second
	processBackOffTime   = uint64(1) // second
)

var (
	CoreBlockReward = big.NewInt(3e+18)        // Block reward in wei for successfully mining a block
	uncleHash       = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	diffInTurn      = big.NewInt(2)            // Block difficulty for in-turn signatures
	diffNoTurn      = big.NewInt(1)            // Block difficulty for out-of-turn signatures

	doubleSignCounter = metrics.NewRegisteredCounter("satoshi/doublesign", nil)

	systemContracts = map[common.Address]bool{
		common.HexToAddress(systemcontracts.ValidatorContract):       true,
		common.HexToAddress(systemcontracts.SlashContract):           true,
		common.HexToAddress(systemcontracts.SystemRewardContract):    true,
		common.HexToAddress(systemcontracts.LightClientContract):     true,
		common.HexToAddress(systemcontracts.RelayerHubContract):      true,
		common.HexToAddress(systemcontracts.CandidateHubContract):    true,
		common.HexToAddress(systemcontracts.GovHubContract):          true,
		common.HexToAddress(systemcontracts.PledgeCandidateContract): true,
		common.HexToAddress(systemcontracts.BurnContract):            true,
		common.HexToAddress(systemcontracts.FoundationContract):      true,
		common.HexToAddress(systemcontracts.StakeHubContract):        true,
		common.HexToAddress(systemcontracts.CoreAgentContract):       true,
		common.HexToAddress(systemcontracts.HashAgentContract):       true,
		common.HexToAddress(systemcontracts.BTCAgentContract):        true,
		common.HexToAddress(systemcontracts.BTCStakeContract):        true,
		common.HexToAddress(systemcontracts.BTCLSTStakeContract):     true,
		common.HexToAddress(systemcontracts.BTCLSTTokenContract):     true,
	}
)

// Various error messages to mark blocks invalid. These should be private to
// prevent engine specific errors from being referenced in the remainder of the
// codebase, inherently breaking if the engine is swapped out. Please put common
// error types into the consensus package.
var (
	// errUnknownBlock is returned when the list of validators is requested for a block
	// that is not part of the local blockchain.
	errUnknownBlock = errors.New("unknown block")

	// errMissingVanity is returned if a block's extra-data section is shorter than
	// 32 bytes, which is required to store the signer vanity.
	errMissingVanity = errors.New("extra-data 32 byte vanity prefix missing")

	// errMissingSignature is returned if a block's extra-data section doesn't seem
	// to contain a 65 byte secp256k1 signature.
	errMissingSignature = errors.New("extra-data 65 byte signature suffix missing")

	// errExtraValidators is returned if non-sprint-end block contain validator data in
	// their extra-data fields.
	errExtraValidators = errors.New("non-sprint-end block contains extra validator list")

	// errInvalidSpanValidators is returned if a block contains an
	// invalid list of validators (i.e. non divisible by 20 bytes).
	errInvalidSpanValidators = errors.New("invalid validator list on sprint end block")

	// errInvalidMixDigest is returned if a block's mix digest is non-zero.
	errInvalidMixDigest = errors.New("non-zero mix digest")

	// errInvalidUncleHash is returned if a block contains an non-empty uncle list.
	errInvalidUncleHash = errors.New("non empty uncle hash")

	// errMismatchingEpochValidators is returned if a sprint block contains a
	// list of validators different than the one the local node calculated.
	errMismatchingEpochValidators = errors.New("mismatching validator list on epoch block")

	// errInvalidDifficulty is returned if the difficulty of a block is missing.
	errInvalidDifficulty = errors.New("invalid difficulty")

	// errWrongDifficulty is returned if the difficulty of a block doesn't match the
	// turn of the signer.
	errWrongDifficulty = errors.New("wrong difficulty")

	// errOutOfRangeChain is returned if an authorization list is attempted to
	// be modified via out-of-range or non-contiguous headers.
	errOutOfRangeChain = errors.New("out of range or non-contiguous chain")

	// errBlockHashInconsistent is returned if an authorization list is attempted to
	// insert an inconsistent block.
	errBlockHashInconsistent = errors.New("the block hash is inconsistent")

	// errUnauthorizedValidator is returned if a header is signed by a non-authorized entity.
	errUnauthorizedValidator = func(val string) error {
		return errors.New("unauthorized validator: " + val)
	}

	// errCoinBaseMisMatch is returned if a header's coinbase do not match with signature
	errCoinBaseMisMatch = errors.New("coinbase do not match with signature")

	// errRecentlySigned is returned if a header is signed by an authorized entity
	// that already signed a header recently, thus is temporarily not allowed to.
	errRecentlySigned = errors.New("recently signed")
)

// SignerFn is a signer callback function to request a header to be signed by a
// backing account.
type SignerFn func(accounts.Account, string, []byte) ([]byte, error)
type SignerTxFn func(accounts.Account, *types.Transaction, *big.Int) (*types.Transaction, error)

func isToSystemContract(to common.Address) bool {
	return systemContracts[to]
}

// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header, sigCache *lru.ARCCache, chainId *big.Int) (common.Address, error) {
	// If the signature's already cached, return that
	hash := header.Hash()
	if address, known := sigCache.Get(hash); known {
		return address.(common.Address), nil
	}
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, errMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]

	// Recover the public key and the Ethereum address
	pubkey, err := crypto.Ecrecover(types.SealHash(header, chainId).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	sigCache.Add(hash, signer)
	return signer, nil
}

// SatoshiRLP returns the rlp bytes which needs to be signed for the satoshi
// sealing. The RLP to sign consists of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func SatoshiRLP(header *types.Header, chainId *big.Int) []byte {
	b := new(bytes.Buffer)
	types.EncodeSigHeader(b, header, chainId)
	return b.Bytes()
}

// Satoshi is the consensus engine of Core Chain
type Satoshi struct {
	chainConfig *params.ChainConfig   // Chain config
	config      *params.SatoshiConfig // Consensus engine configuration parameters for satoshi consensus
	genesisHash common.Hash
	db          ethdb.Database // Database to store and retrieve snapshot checkpoints

	recentSnaps   *lru.ARCCache // Snapshots for recent block to speed up
	signatures    *lru.ARCCache // Signatures of recent blocks to speed up mining
	recentHeaders *lru.ARCCache //
	// Recent headers to check for double signing: key includes block number and miner. value is the block header
	// If same key's value already exists for different block header roots then double sign is detected

	signer types.Signer

	val      common.Address // Ethereum address of the signing key
	signFn   SignerFn       // Signer function to authorize hashes with
	signTxFn SignerTxFn

	lock sync.RWMutex // Protects the signer fields

	ethAPI          *ethapi.BlockChainAPI
	validatorSetABI abi.ABI
	slashABI        abi.ABI
	candidateHubABI abi.ABI

	// The fields below are for testing only
	fakeDiff bool // Skip difficulty verifications
}

// New creates a Satoshi consensus engine.
func New(
	chainConfig *params.ChainConfig,
	db ethdb.Database,
	ethAPI *ethapi.BlockChainAPI,
	genesisHash common.Hash,
) *Satoshi {
	// get satoshi config
	satoshiConfig := chainConfig.Satoshi
	log.Info("satoshi", "chainConfig", chainConfig)

	// Set any missing consensus parameters to their defaults
	if satoshiConfig != nil {
		if satoshiConfig.Epoch == 0 {
			satoshiConfig.Epoch = defaultEpochLength
		}
		if satoshiConfig.Round == 0 {
			satoshiConfig.Round = defaultRoundInterval
		}
	}

	// Allocate the snapshot caches and create the engine
	recentSnaps, err := lru.NewARC(inMemorySnapshots)
	if err != nil {
		panic(err)
	}
	signatures, err := lru.NewARC(inMemorySignatures)
	if err != nil {
		panic(err)
	}
	recentHeaders, err := lru.NewARC(inMemoryHeaders)
	if err != nil {
		panic(err)
	}
	vABI, err := abi.JSON(strings.NewReader(validatorSetABI))
	if err != nil {
		panic(err)
	}
	sABI, err := abi.JSON(strings.NewReader(slashABI))
	if err != nil {
		panic(err)
	}
	cABI, err := abi.JSON(strings.NewReader(candidateHubABI))
	if err != nil {
		panic(err)
	}
	c := &Satoshi{
		chainConfig:     chainConfig,
		config:          satoshiConfig,
		genesisHash:     genesisHash,
		db:              db,
		ethAPI:          ethAPI,
		recentSnaps:     recentSnaps,
		recentHeaders:   recentHeaders,
		signatures:      signatures,
		validatorSetABI: vABI,
		slashABI:        sABI,
		candidateHubABI: cABI,
		signer:          types.LatestSigner(chainConfig),
	}

	return c
}

func (p *Satoshi) IsSystemTransaction(tx *types.Transaction, header *types.Header) (bool, error) {
	// deploy a contract
	if tx.To() == nil {
		return false, nil
	}
	sender, err := types.Sender(p.signer, tx)
	if err != nil {
		return false, errors.New("UnAuthorized transaction")
	}
	if sender == header.Coinbase && isToSystemContract(*tx.To()) && tx.GasPrice().Cmp(big.NewInt(0)) == 0 {
		return true, nil
	}
	return false, nil
}

func (p *Satoshi) IsSystemContract(to *common.Address) bool {
	if to == nil {
		return false
	}
	return isToSystemContract(*to)
}

// Author implements consensus.Engine, returning the SystemAddress
func (p *Satoshi) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// getParent returns the parent of a given block.
func (p *Satoshi) getParent(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) (*types.Header, error) {
	var parent *types.Header
	number := header.Number.Uint64()
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}

	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return nil, consensus.ErrUnknownAncestor
	}
	return parent, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (p *Satoshi) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header) error {
	return p.verifyHeader(chain, header, nil)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (p *Satoshi) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	gopool.Submit(func() {
		for i, header := range headers {
			err := p.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	})
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (p *Satoshi) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}
	number := header.Number.Uint64()

	// Don't waste time checking blocks from the future
	if header.Time > uint64(time.Now().Unix()) {
		return consensus.ErrFutureBlock
	}
	// Check that the extra-data contains the vanity, validators and signature.
	if len(header.Extra) < extraVanity {
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		return errMissingSignature
	}
	// check extra data
	isEpoch := number%p.config.Epoch == 0

	// Ensure that the extra-data contains a signer list on checkpoint, but none otherwise
	signersBytes := len(header.Extra) - extraVanity - extraSeal
	if !isEpoch && signersBytes != 0 {
		return errExtraValidators
	}

	if isEpoch && signersBytes%validatorBytesLength != 0 {
		return errInvalidSpanValidators
	}

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != (common.Hash{}) {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in PoA
	if header.UncleHash != uncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if number > 0 {
		if header.Difficulty == nil {
			return errInvalidDifficulty
		}
	}

	parent, err := p.getParent(chain, header, parents)
	if err != nil {
		return err
	}

	// Verify the block's gas usage and (if applicable) verify the base fee.
	if !chain.Config().IsLondon(header.Number) {
		// Verify BaseFee not present before EIP-1559 fork.
		if header.BaseFee != nil {
			return fmt.Errorf("invalid baseFee before fork: have %d, expected 'nil'", header.BaseFee)
		}
	} else if err := eip1559.VerifyEIP1559Header(chain.Config(), parent, header); err != nil {
		// Verify the header's EIP-1559 attributes.
		return err
	}

	cancun := chain.Config().IsCancun(header.Number, header.Time)
	if !cancun {
		switch {
		case header.ExcessBlobGas != nil:
			return fmt.Errorf("invalid excessBlobGas: have %d, expected nil", header.ExcessBlobGas)
		case header.BlobGasUsed != nil:
			return fmt.Errorf("invalid blobGasUsed: have %d, expected nil", header.BlobGasUsed)
		case header.ParentBeaconRoot != nil:
			return fmt.Errorf("invalid parentBeaconRoot, have %#x, expected nil", header.ParentBeaconRoot)
		case header.WithdrawalsHash != nil:
			return fmt.Errorf("invalid WithdrawalsHash, have %#x, expected nil", header.WithdrawalsHash)
		}
	} else {
		switch {
		case header.ParentBeaconRoot != nil:
			return fmt.Errorf("invalid parentBeaconRoot, have %#x, expected nil", header.ParentBeaconRoot)
		case !header.EmptyWithdrawalsHash():
			return errors.New("header has wrong WithdrawalsHash")
		}
		if err := eip4844.VerifyEIP4844Header(parent, header); err != nil {
			return err
		}
	}

	// All basic checks passed, verify cascading fields
	return p.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (p *Satoshi) verifyCascadingFields(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}

	parent, err := p.getParent(chain, header, parents)
	if err != nil {
		return err
	}

	snap, err := p.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// blockTimeVerify
	if header.Time < parent.Time+p.config.Period+p.backOffTime(snap, header, header.Coinbase) {
		return consensus.ErrFutureBlock
	}

	// Verify that the gas limit is <= 2^63-1
	capacity := uint64(0x7fffffffffffffff)
	if header.GasLimit > capacity {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, capacity)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, want %d += %d", header.GasLimit, parent.GasLimit, limit-1)
	}

	// All basic checks passed, verify the seal and return
	return p.verifySeal(chain, header, parents)
}

// snapshot retrieves the authorization snapshot at a given point in time.
func (p *Satoshi) snapshot(chain consensus.ChainHeaderReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk for checkpoints
	var (
		headers []*types.Header
		snap    *Snapshot
	)

	for snap == nil {
		// If an in-memory snapshot was found, use that
		if s, ok := p.recentSnaps.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}

		// If an on-disk checkpoint snapshot can be found, use that
		if number%checkpointInterval == 0 {
			if s, err := loadSnapshot(p.config, p.signatures, p.db, hash, p.ethAPI); err == nil {
				log.Trace("Loaded snapshot from disk", "number", number, "hash", hash)
				snap = s
				break
			}
		}

		// If we're at the genesis, snapshot the initial state.
		if number == 0 {
			checkpoint := chain.GetHeaderByNumber(number)
			if checkpoint != nil {
				// get checkpoint data
				hash := checkpoint.Hash()

				if len(checkpoint.Extra) <= extraVanity+extraSeal {
					return nil, errors.New("invalid extra-data for genesis block, check the genesis.json file")
				}
				validatorBytes := checkpoint.Extra[extraVanity : len(checkpoint.Extra)-extraSeal]
				// get validators from headers
				validators, err := ParseValidators(validatorBytes)
				if err != nil {
					return nil, err
				}

				// new snap shot
				snap = newSnapshot(p.config, p.signatures, number, hash, validators, p.ethAPI)
				if err := snap.store(p.db); err != nil {
					return nil, err
				}
				log.Info("Stored checkpoint snapshot to disk", "number", number, "hash", hash)
				break
			}
		}

		// No snapshot for this header, gather the header and move backward
		var header *types.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			header = parents[len(parents)-1]
			if header.Hash() != hash || header.Number.Uint64() != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			header = chain.GetHeader(hash, number)
			if header == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}
		headers = append(headers, header)
		number, hash = number-1, header.ParentHash
	}

	// check if snapshot is nil
	if snap == nil {
		return nil, fmt.Errorf("unknown error while retrieving snapshot at block number %v", number)
	}

	// Previous snapshot found, apply any pending headers on top of it
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}

	snap, err := snap.apply(headers, chain, parents, p.chainConfig.ChainID)
	if err != nil {
		return nil, err
	}
	p.recentSnaps.Add(snap.Hash, snap)

	// If we've generated a new checkpoint snapshot, save to disk
	if snap.Number%checkpointInterval == 0 && len(headers) > 0 {
		if err = snap.store(p.db); err != nil {
			return nil, err
		}
		log.Trace("Stored snapshot to disk", "number", snap.Number, "hash", snap.Hash)
	}
	return snap, err
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (p *Satoshi) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// VerifySeal implements consensus.Engine, checking whether the signature contained
// in the header satisfies the consensus protocol requirements.
func (p *Satoshi) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return p.verifySeal(chain, header, nil)
}

// verifySeal checks whether the signature contained in the header satisfies the
// consensus protocol requirements. The method accepts an optional list of parent
// headers that aren't yet part of the local blockchain to generate the snapshots
// from.
func (p *Satoshi) verifySeal(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := p.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// Resolve the authorization key and check against validators
	signer, err := ecrecover(header, p.signatures, p.chainConfig.ChainID)
	if err != nil {
		return err
	}

	if signer != header.Coinbase {
		return errCoinBaseMisMatch
	}

	// check for double sign & add to cache
	key := proposalKey(*header)
	preHash, ok := p.recentHeaders.Get(key)
	if ok && preHash != header.Hash() {
		doubleSignCounter.Inc(1)
		log.Warn("DoubleSign detected", " block", header.Number, " miner", header.Coinbase,
			"hash1", preHash.(common.Hash), "hash2", header.Hash())
	} else {
		p.recentHeaders.Add(key, header.Hash())
	}

	if _, ok := snap.Validators[signer]; !ok {
		return errUnauthorizedValidator(signer.String())
	}

	for seen, recent := range snap.Recents {
		if recent == signer {
			// Signer is among recents, only fail if the current block doesn't shift it out
			if limit := uint64(len(snap.Validators)/2 + 1); seen > number-limit {
				return errRecentlySigned
			}
		}
	}

	// Ensure that the difficulty corresponds to the turn-ness of the signer
	if !p.fakeDiff {
		inturn := snap.inturn(signer)
		if inturn && header.Difficulty.Cmp(diffInTurn) != 0 {
			return errWrongDifficulty
		}
		if !inturn && header.Difficulty.Cmp(diffNoTurn) != 0 {
			return errWrongDifficulty
		}
	}

	return nil
}

// NextInTurnValidator return the next in-turn validator for header
func (p *Satoshi) NextInTurnValidator(chain consensus.ChainHeaderReader, header *types.Header) (common.Address, error) {
	snap, err := p.snapshot(chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return common.Address{}, err
	}

	return snap.inturnValidator(), nil
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (p *Satoshi) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	header.Coinbase = p.val
	header.Nonce = types.BlockNonce{}

	number := header.Number.Uint64()
	snap, err := p.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	// Set the correct difficulty
	header.Difficulty = CalcDifficulty(snap, p.val)

	// Ensure the extra data has all it's components
	if len(header.Extra) < extraVanity-nextForkHashSize {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity-nextForkHashSize-len(header.Extra))...)
	}

	// Ensure the timestamp has the correct delay
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Time = parent.Time + p.config.Period + p.backOffTime(snap, header, p.val)
	if header.Time < uint64(time.Now().Unix()) {
		header.Time = uint64(time.Now().Unix())
	}

	header.Extra = header.Extra[:extraVanity-nextForkHashSize]
	nextForkHash := forkid.NextForkHash(p.chainConfig, p.genesisHash, chain.GenesisHeader().Time, number, header.Time)
	header.Extra = append(header.Extra, nextForkHash[:]...)

	if number%p.config.Epoch == 0 {
		newValidators, err := p.getCurrentValidators(header.ParentHash)
		if err != nil {
			return err
		}
		// sort validator by address
		sort.Sort(validatorsAscending(newValidators))
		for _, validator := range newValidators {
			header.Extra = append(header.Extra, validator.Bytes()...)
		}
	}

	// add extra seal space
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)

	// Mix digest is reserved for now, set to empty
	header.MixDigest = common.Hash{}

	return nil
}

func (p *Satoshi) BeforeValidateTx(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, txs *[]*types.Transaction,
	uncles []*types.Header, receipts *[]*types.Receipt, systemTxs *[]*types.Transaction, usedGas *uint64, tracer *tracing.Hooks) (err error) {
	cx := chainContext{Chain: chain, satoshi: p}

	parent := chain.GetHeaderByHash(header.ParentHash)
	if p.chainConfig.IsOnDemeter(header.Number, parent.Time, header.Time) {
		contracts := []string{
			systemcontracts.StakeHubContract,
			systemcontracts.CoreAgentContract,
			systemcontracts.HashAgentContract,
			systemcontracts.BTCAgentContract,
			systemcontracts.BTCStakeContract,
			systemcontracts.BTCLSTStakeContract,
			systemcontracts.BTCLSTTokenContract,
		}

		err := p.initContractWithContracts(state, header, cx, txs, receipts, systemTxs, usedGas, false, contracts, tracer)
		if err != nil {
			log.Error("init contract failed on demeter fork")
		}
	}

	// If the block is the last one in a round, execute turn round to update the validator set.
	if p.isRoundEnd(chain, header) {
		// try turnRound
		log.Trace("turn round", "block hash", header.Hash())
		err = p.turnRound(state, header, cx, txs, receipts, systemTxs, usedGas, false, tracer)
		if err != nil {
			// it is possible that turn round failed.
			log.Error("turn round failed", "block hash", header.Hash())
		}
	}
	return
}

func (p *Satoshi) BeforePackTx(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB,
	txs *[]*types.Transaction, uncles []*types.Header, receipts *[]*types.Receipt, tracer *tracing.Hooks) (err error) {
	cx := chainContext{Chain: chain, satoshi: p}

	parent := chain.GetHeaderByHash(header.ParentHash)
	if p.chainConfig.IsOnDemeter(header.Number, parent.Time, header.Time) {
		contracts := []string{
			systemcontracts.StakeHubContract,
			systemcontracts.CoreAgentContract,
			systemcontracts.HashAgentContract,
			systemcontracts.BTCAgentContract,
			systemcontracts.BTCStakeContract,
			systemcontracts.BTCLSTStakeContract,
			systemcontracts.BTCLSTTokenContract,
		}

		err := p.initContractWithContracts(state, header, cx, txs, receipts, nil, &header.GasUsed, true, contracts, tracer)
		if err != nil {
			log.Error("init contract failed on demeter fork")
		}
	}

	// If the block is the last one in a round, execute turn round to update the validator set.
	if p.isRoundEnd(chain, header) {
		// try turnRound
		log.Trace("turn round", "block hash", header.Hash())
		err = p.turnRound(state, header, cx, txs, receipts, nil, &header.GasUsed, true, tracer)
		if err != nil {
			// it is possible that turn round failed.
			log.Error("turn round failed", "block hash", header.Hash())
		}
	}
	return
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given.
func (p *Satoshi) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state vm.StateDB, txs *[]*types.Transaction,
	uncles []*types.Header, _ []*types.Withdrawal, receipts *[]*types.Receipt, systemTxs *[]*types.Transaction, usedGas *uint64, tracer *tracing.Hooks) error {
	// warn if not in majority fork
	number := header.Number.Uint64()
	snap, err := p.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}
	nextForkHash := forkid.NextForkHash(p.chainConfig, p.genesisHash, chain.GenesisHeader().Time, number, header.Time)
	if !snap.isMajorityFork(hex.EncodeToString(nextForkHash[:])) {
		log.Debug("there is a possible fork, and your client is not the majority. Please check...", "nextForkHash", hex.EncodeToString(nextForkHash[:]))
	}
	// If the block is a epoch end block, verify the validator list
	// The verification can only be done when the state is ready, it can't be done in VerifyHeader.
	if header.Number.Uint64()%p.config.Epoch == 0 {
		newValidators, err := p.getCurrentValidators(header.ParentHash)
		if err != nil {
			return err
		}
		// sort validator by address
		sort.Sort(validatorsAscending(newValidators))
		validatorsBytes := make([]byte, len(newValidators)*validatorBytesLength)
		for i, validator := range newValidators {
			copy(validatorsBytes[i*validatorBytesLength:], validator.Bytes())
		}

		extraSuffix := len(header.Extra) - extraSeal
		if !bytes.Equal(header.Extra[extraVanity:extraSuffix], validatorsBytes) {
			return errMismatchingEpochValidators
		}
	}
	// No block rewards in PoA, so the state remains as is and uncles are dropped
	cx := chainContext{Chain: chain, satoshi: p}
	if header.Number.Cmp(common.Big1) == 0 {
		err := p.initContract(state, header, cx, txs, receipts, systemTxs, usedGas, false, tracer)
		if err != nil {
			log.Error("init contract failed")
		}
	}
	if header.Difficulty.Cmp(diffInTurn) != 0 {
		spoiledVal := snap.supposeValidator()
		signedRecently := false
		for _, recent := range snap.Recents {
			if recent == spoiledVal {
				signedRecently = true
				break
			}
		}
		if !signedRecently {
			log.Trace("slash validator", "block hash", header.Hash(), "address", spoiledVal)
			err = p.slash(spoiledVal, state, header, cx, txs, receipts, systemTxs, usedGas, false, tracer)
			if err != nil {
				// it is possible that slash validator failed because of the slash channel is disabled.
				log.Error("slash validator failed", "block hash", header.Hash(), "address", spoiledVal, "err", err.Error())
			}
		}
	}
	val := header.Coinbase
	err = p.distributeIncoming(val, state, header, cx, txs, receipts, systemTxs, usedGas, false, tracer)
	if err != nil {
		return err
	}
	if len(*systemTxs) > 0 {
		return errors.New("the length of systemTxs do not match")
	}
	return nil
}

// FinalizeAndAssemble implements consensus.Engine, ensuring no uncles are set,
// nor block rewards given, and returns the final block.
func (p *Satoshi) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt, _ []*types.Withdrawal, tracer *tracing.Hooks) (*types.Block, []*types.Receipt, error) {
	// No block rewards in PoA, so the state remains as is and uncles are dropped
	cx := chainContext{Chain: chain, satoshi: p}

	if bc, ok := chain.(*core.BlockChain); ok {
		cx.feemarket = bc.FeeMarket()
	}
	if txs == nil {
		txs = make([]*types.Transaction, 0)
	}
	if receipts == nil {
		receipts = make([]*types.Receipt, 0)
	}
	if header.Number.Cmp(common.Big1) == 0 {
		err := p.initContract(state, header, cx, &txs, &receipts, nil, &header.GasUsed, true, tracer)
		if err != nil {
			log.Error("init contract failed")
		}
	}
	if header.Difficulty.Cmp(diffInTurn) != 0 {
		number := header.Number.Uint64()
		snap, err := p.snapshot(chain, number-1, header.ParentHash, nil)
		if err != nil {
			return nil, nil, err
		}
		spoiledVal := snap.supposeValidator()
		signedRecently := false
		for _, recent := range snap.Recents {
			if recent == spoiledVal {
				signedRecently = true
				break
			}
		}
		if !signedRecently {
			err = p.slash(spoiledVal, state, header, cx, &txs, &receipts, nil, &header.GasUsed, true, tracer)
			if err != nil {
				// it is possible that slash validator failed because of the slash channel is disabled.
				log.Error("slash validator failed", "block hash", header.Hash(), "address", spoiledVal, "err", err.Error())
			}
		}
	}
	err := p.distributeIncoming(p.val, state, header, cx, &txs, &receipts, nil, &header.GasUsed, true, tracer)
	if err != nil {
		return nil, nil, err
	}
	// should not happen. Once happen, stop the node is better than broadcast the block
	if header.GasLimit < header.GasUsed {
		return nil, nil, errors.New("gas consumption of system txs exceed the gas limit")
	}
	header.UncleHash = types.CalcUncleHash(nil)
	var blk *types.Block
	var rootHash common.Hash
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		rootHash = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
		wg.Done()
	}()
	go func() {
		blk = types.NewBlock(header, txs, nil, receipts, trie.NewStackTrie(nil))
		wg.Done()
	}()
	wg.Wait()
	blk.SetRoot(rootHash)
	// Assemble and return the final block for sealing
	return blk, receipts, nil
}

// Authorize injects a private key into the consensus engine to mint new blocks
// with.
func (p *Satoshi) Authorize(val common.Address, signFn SignerFn, signTxFn SignerTxFn) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.val = val
	p.signFn = signFn
	p.signTxFn = signTxFn
}

// Argument leftOver is the time reserved for block finalize(calculate root, distribute income...)
func (p *Satoshi) Delay(chain consensus.ChainReader, header *types.Header, leftOver *time.Duration) *time.Duration {
	delay := time.Until(time.Unix(int64(header.Time), 0))

	if *leftOver >= time.Duration(p.config.Period)*time.Second {
		// ignore invalid leftOver
		log.Error("Delay invalid argument", "leftOver", leftOver.String(), "Period", p.config.Period)
	} else if *leftOver >= delay {
		delay = time.Duration(0)
		return &delay
	} else {
		delay = delay - *leftOver
	}

	// The blocking time should be no more than half of period
	half := time.Duration(p.config.Period) * time.Second / 2
	if delay > half {
		delay = half
	}
	return &delay
}

// Seal implements consensus.Engine, attempting to create a sealed block using
// the local signing credentials.
func (p *Satoshi) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	header := block.Header()

	// Sealing the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	// For 0-period chains, refuse to seal empty blocks (no reward but would spin sealing)
	if p.config.Period == 0 && len(block.Transactions()) == 0 {
		log.Info("Sealing paused, waiting for transactions")
		return nil
	}
	// Don't hold the val fields for the entire sealing procedure
	p.lock.RLock()
	val, signFn := p.val, p.signFn
	p.lock.RUnlock()

	snap, err := p.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	// Bail out if we're unauthorized to sign a block
	if _, authorized := snap.Validators[val]; !authorized {
		return errUnauthorizedValidator(val.String())
	}

	// If we're amongst the recent signers, wait for the next block
	for seen, recent := range snap.Recents {
		if recent == val {
			// Signer is among recents, only wait if the current block doesn't shift it out
			if limit := uint64(len(snap.Validators)/2 + 1); number < limit || seen > number-limit {
				log.Info("Signed recently, must wait for others")
				return nil
			}
		}
	}

	// Sweet, the protocol permits us to sign the block, wait for our time
	delay := time.Until(time.Unix(int64(header.Time), 0))

	log.Info("Sealing block with", "number", number, "delay", delay, "headerDifficulty", header.Difficulty, "val", val.Hex())

	// Sign all the things!
	sig, err := signFn(accounts.Account{Address: val}, accounts.MimetypeSatoshi, SatoshiRLP(header, p.chainConfig.ChainID))
	if err != nil {
		return err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sig)

	// Wait until sealing is terminated or delay timeout.
	log.Trace("Waiting for slot to sign and propagate", "delay", common.PrettyDuration(delay))
	go func() {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}
		if p.shouldWaitForCurrentBlockProcess(chain, header, snap) {
			log.Info("Waiting for received in turn block to process")
			select {
			case <-stop:
				log.Info("Received block process finished, abort block seal")
				return
			case <-time.After(time.Duration(processBackOffTime) * time.Second):
				if chain.CurrentHeader().Number.Uint64() >= header.Number.Uint64() {
					log.Info("Process backoff time exhausted, and current header has updated to abort this seal")
					return
				}
				log.Info("Process backoff time exhausted, start to seal block")
			}
		}

		select {
		case results <- block.WithSeal(header):
		default:
			log.Warn("Sealing result is not read by miner", "sealhash", types.SealHash(header, p.chainConfig.ChainID))
		}
	}()

	return nil
}

func (p *Satoshi) shouldWaitForCurrentBlockProcess(chain consensus.ChainHeaderReader, header *types.Header, snap *Snapshot) bool {
	if header.Difficulty.Cmp(diffInTurn) == 0 {
		return false
	}

	highestVerifiedHeader := chain.GetHighestVerifiedHeader()
	if highestVerifiedHeader == nil {
		return false
	}

	if header.ParentHash == highestVerifiedHeader.ParentHash {
		return true
	}
	return false
}

func (p *Satoshi) EnoughDistance(chain consensus.ChainReader, header *types.Header) bool {
	snap, err := p.snapshot(chain, header.Number.Uint64()-1, header.ParentHash, nil)
	if err != nil {
		return true
	}
	return snap.enoughDistance(p.val, header)
}

func (p *Satoshi) IsLocalBlock(header *types.Header) bool {
	return p.val == header.Coinbase
}

func (p *Satoshi) SignRecently(chain consensus.ChainReader, parent *types.Block) (bool, error) {
	snap, err := p.snapshot(chain, parent.NumberU64(), parent.Hash(), nil)
	if err != nil {
		return true, err
	}

	// Bail out if we're unauthorized to sign a block
	if _, authorized := snap.Validators[p.val]; !authorized {
		return true, errUnauthorizedValidator(p.val.String())
	}

	// If we're amongst the recent signers, wait for the next block
	number := parent.NumberU64() + 1
	for seen, recent := range snap.Recents {
		if recent == p.val {
			// Signer is among recents, only wait if the current block doesn't shift it out
			if limit := uint64(len(snap.Validators)/2 + 1); number < limit || seen > number-limit {
				return true, nil
			}
		}
	}
	return false, nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (p *Satoshi) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	snap, err := p.snapshot(chain, parent.Number.Uint64(), parent.Hash(), nil)
	if err != nil {
		return nil
	}
	return CalcDifficulty(snap, p.val)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func CalcDifficulty(snap *Snapshot, signer common.Address) *big.Int {
	if snap.inturn(signer) {
		return new(big.Int).Set(diffInTurn)
	}
	return new(big.Int).Set(diffNoTurn)
}

// SealHash returns the hash of a block prior to it being sealed.
func (p *Satoshi) SealHash(header *types.Header) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	types.EncodeSigHeaderWithoutVoteAttestation(hasher, header, p.chainConfig.ChainID)
	hasher.Sum(hash[:0])
	return hash
}

// APIs implements consensus.Engine, returning the user facing RPC API to query snapshot.
func (p *Satoshi) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "satoshi",
		Version:   "1.0",
		Service:   &API{chain: chain, satoshi: p},
		Public:    false,
	}}
}

// Close implements consensus.Engine. It's a noop for satoshi as there are no background threads.
func (p *Satoshi) Close() error {
	return nil
}

func (p *Satoshi) IsRoundEnd(chain consensus.ChainHeaderReader, header *types.Header) bool {
	number := header.Number.Uint64()
	snapshot, _ := p.snapshot(chain, number-1, header.ParentHash, nil)
	if snapshot == nil {
		return true
	}
	if number > 0 && number%snapshot.config.Epoch == uint64(len(snapshot.Validators)/2) {
		// find the header of last block in the previous round
		checkHeader := FindAncientHeader(header, uint64(len(snapshot.Validators)/2)+1, chain, nil)
		if checkHeader != nil {
			return p.isRoundEnd(chain, checkHeader)
		}
	}
	return false
}

// ==========================  interaction with contract/account =========

// getCurrentValidators get current validators
func (p *Satoshi) getCurrentValidators(blockHash common.Hash) ([]common.Address, error) {
	// block
	blockNr := rpc.BlockNumberOrHashWithHash(blockHash, false)

	// method
	method := "getValidators"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // cancel when we are finished consuming integers

	data, err := p.validatorSetABI.Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for getValidators", "error", err)
		return nil, err
	}
	// call
	msgData := (hexutil.Bytes)(data)
	toAddress := common.HexToAddress(systemcontracts.ValidatorContract)
	gas := (hexutil.Uint64)(uint64(math.MaxUint64 / 2))
	result, err := p.ethAPI.Call(ctx, ethapi.TransactionArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, &blockNr, nil, nil)
	if err != nil {
		return nil, err
	}

	var (
		ret0 = new([]common.Address)
	)
	out := ret0

	if err := p.validatorSetABI.UnpackIntoInterface(out, method, result); err != nil {
		return nil, err
	}

	valz := make([]common.Address, len(*ret0))
	// nolint: gosimple
	for i, a := range *ret0 {
		valz[i] = a
	}
	return valz, nil
}

// distributeIncoming collect transaction fees and distribute to validator contract
func (p *Satoshi) distributeIncoming(val common.Address, state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, tracer *tracing.Hooks) error {
	coinbase := header.Coinbase
	balance := state.GetBalance(consensus.SystemAddress)
	state.SetBalance(consensus.SystemAddress, common.U2560, tracing.BalanceDecreaseCoreDistributeReward)
	state.AddBalance(coinbase, balance, tracing.BalanceIncreaseCoreDistributeReward)
	log.Trace("distribute to validator contract", "block hash", header.Hash(), "amount", balance)
	return p.distributeToValidator(balance.ToBig(), val, state, header, chain, txs, receipts, receivedTxs, usedGas, mining, tracer)
}

// slash spoiled validators
func (p *Satoshi) slash(spoiledVal common.Address, state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, tracer *tracing.Hooks) error {
	// method
	method := "slash"
	// get packed data
	data, err := p.slashABI.Pack(method,
		spoiledVal,
	)
	if err != nil {
		log.Error("Unable to pack tx for slash", "error", err)
		return err
	}
	// get system message
	msg := p.getSystemMessage(header.Coinbase, common.HexToAddress(systemcontracts.SlashContract), params.SystemTxsGas, data, common.Big0)
	// apply message
	return p.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining, tracer)
}

// turnRound call candidate contract to execute turn round
func (p *Satoshi) turnRound(state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, tracer *tracing.Hooks) error {
	// method
	method := "turnRound"

	// get packed data
	data, err := p.candidateHubABI.Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for turnRound", "error", err)
		return err
	}
	// get system message
	msg := p.getSystemMessage(header.Coinbase, common.HexToAddress(systemcontracts.CandidateHubContract), header.GasLimit-params.SystemTxsGas, data, common.Big0)
	// apply message
	return p.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining, tracer)
}

// init contract
func (p *Satoshi) initContract(state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, tracer *tracing.Hooks) error {
	contracts := []string{
		systemcontracts.ValidatorContract,
		systemcontracts.SlashContract,
		systemcontracts.SystemRewardContract,
		systemcontracts.LightClientContract,
		systemcontracts.RelayerHubContract,
		systemcontracts.CandidateHubContract,
		systemcontracts.GovHubContract,
		systemcontracts.PledgeCandidateContract,
		systemcontracts.BurnContract,
	}

	if p.chainConfig.IsDemeter(header.Number, header.Time) {
		contracts = append(contracts, systemcontracts.StakeHubContract)
		contracts = append(contracts, systemcontracts.CoreAgentContract)
		contracts = append(contracts, systemcontracts.HashAgentContract)
		contracts = append(contracts, systemcontracts.BTCAgentContract)
		contracts = append(contracts, systemcontracts.BTCStakeContract)
		contracts = append(contracts, systemcontracts.BTCLSTStakeContract)
		contracts = append(contracts, systemcontracts.BTCLSTTokenContract)
	}

	return p.initContractWithContracts(state, header, chain, txs, receipts, receivedTxs, usedGas, mining, contracts, tracer)
}

func (p *Satoshi) initContractWithContracts(state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, contracts []string, tracer *tracing.Hooks) error {
	// method
	method := "init"

	// get packed data
	data, err := p.validatorSetABI.Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for init validator set", "error", err)
		return err
	}
	for _, c := range contracts {
		msg := p.getSystemMessage(header.Coinbase, common.HexToAddress(c), header.GasLimit, data, common.Big0)
		// apply message
		log.Trace("init contract", "block hash", header.Hash(), "contract", c)
		err = p.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining, tracer)
		if err != nil {
			return err
		}
	}
	return nil
}

// distributeToValidator call validator contract to distribute reward
func (p *Satoshi) distributeToValidator(amount *big.Int, validator common.Address,
	state vm.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool, tracer *tracing.Hooks) error {
	// method
	method := "deposit"

	// get packed data
	data, err := p.validatorSetABI.Pack(method,
		validator,
	)
	if err != nil {
		log.Error("Unable to pack tx for deposit", "error", err)
		return err
	}
	// get system message
	msg := p.getSystemMessage(header.Coinbase, common.HexToAddress(systemcontracts.ValidatorContract), params.SystemTxsGas, data, amount)
	// apply message
	return p.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining, tracer)
}

// get system message
func (p *Satoshi) getSystemMessage(from, toAddress common.Address, gas uint64, data []byte, value *big.Int) *core.Message {
	return &core.Message{
		From:     from,
		GasLimit: gas,
		GasPrice: big.NewInt(0),
		Value:    value,
		To:       &toAddress,
		Data:     data,
	}
}

func (p *Satoshi) applyTransaction(
	msg *core.Message,
	state vm.StateDB,
	header *types.Header,
	chainContext core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt,
	receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool,
	tracer *tracing.Hooks,
) (applyErr error) {
	nonce := state.GetNonce(msg.From)
	expectedTx := types.NewTransaction(nonce, *msg.To, msg.Value, msg.GasLimit, msg.GasPrice, msg.Data)
	expectedHash := p.signer.Hash(expectedTx)

	if msg.From == p.val && mining {
		var err error
		expectedTx, err = p.signTxFn(accounts.Account{Address: msg.From}, expectedTx, p.chainConfig.ChainID)
		if err != nil {
			return err
		}
	} else {
		if receivedTxs == nil || len(*receivedTxs) == 0 || (*receivedTxs)[0] == nil {
			return errors.New("supposed to get a actual transaction, but get none")
		}
		actualTx := (*receivedTxs)[0]
		if !bytes.Equal(p.signer.Hash(actualTx).Bytes(), expectedHash.Bytes()) {
			return fmt.Errorf("expected tx hash %v, get %v, nonce %d, to %s, value %s, gas %d, gasPrice %s, data %s", expectedHash.String(), actualTx.Hash().String(),
				expectedTx.Nonce(),
				expectedTx.To().String(),
				expectedTx.Value().String(),
				expectedTx.Gas(),
				expectedTx.GasPrice().String(),
				hex.EncodeToString(expectedTx.Data()),
			)
		}
		expectedTx = actualTx
		// move to next
		*receivedTxs = (*receivedTxs)[1:]
	}
	state.SetTxContext(expectedTx.Hash(), len(*txs))

	// Create a new context to be used in the EVM environment
	context := core.NewEVMBlockContext(header, chainContext, nil)
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	evm := vm.NewEVM(context, state, p.chainConfig, vm.Config{Tracer: tracer})
	evm.SetTxContext(core.NewEVMTxContext(msg))

	// Tracing receipt will be set if there is no error and will be used to trace the transaction
	var tracingReceipt *types.Receipt
	if tracer != nil {
		if tracer.OnSystemTxStart != nil {
			tracer.OnSystemTxStart()
		}
		if tracer.OnTxStart != nil {
			tracer.OnTxStart(evm.GetVMContext(), expectedTx, msg.From)
		}

		// Defers are last in first out, so OnTxEnd will run before OnSystemTxEnd in this transaction,
		// which is what we want.
		if tracer.OnSystemTxEnd != nil {
			defer func() {
				tracer.OnSystemTxEnd()
			}()
		}
		if tracer.OnTxEnd != nil {
			defer func() {
				tracer.OnTxEnd(tracingReceipt, applyErr)
			}()
		}
	}

	gasUsed, err := applyMessage(msg, evm, state, header, p.chainConfig, chainContext)
	if err != nil {
		log.Error(fmt.Sprintf("Apply system transaction failed, to: %v, calldata: %v, error: %v", expectedTx.To(), expectedTx.Data(), err.Error()))
	}
	*txs = append(*txs, expectedTx)
	var root []byte
	if p.chainConfig.IsByzantium(header.Number) {
		state.Finalise(true)
	} else {
		root = state.IntermediateRoot(p.chainConfig.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gasUsed
	tracingReceipt = types.NewReceipt(root, false, *usedGas)
	tracingReceipt.TxHash = expectedTx.Hash()
	tracingReceipt.GasUsed = gasUsed

	// Set the receipt logs and create a bloom for filtering
	tracingReceipt.Logs = state.GetLogs(expectedTx.Hash(), header.Number.Uint64(), header.Hash())
	tracingReceipt.Bloom = types.CreateBloom(types.Receipts{tracingReceipt})
	tracingReceipt.BlockHash = header.Hash()
	tracingReceipt.BlockNumber = header.Number
	tracingReceipt.TransactionIndex = uint(state.TxIndex())
	*receipts = append(*receipts, tracingReceipt)

	return nil
}

// isRoundEnd returns true if the given header belongs to the last block of the round, otherwise false
func (p *Satoshi) isRoundEnd(chain consensus.ChainHeaderReader, header *types.Header) bool {
	number := header.Number.Uint64()
	if number%p.config.Epoch == p.config.Epoch-1 {
		lastCheckNumber := uint64(1)
		if number > p.config.Epoch {
			lastCheckNumber = number - p.config.Epoch
		}
		lastCheckBlock := chain.GetHeaderByNumber(lastCheckNumber)
		if header.Time/p.config.Round > lastCheckBlock.Time/p.config.Round {
			return true
		}
	}
	return false
}

// ===========================     utility function        ==========================
func (s *Satoshi) backOffTime(snap *Snapshot, header *types.Header, val common.Address) uint64 {
	if snap.inturn(val) {
		return 0
	} else {
		delay := initialBackOffTime
		validators := snap.validators()
		if s.chainConfig.IsZeus(header.Number) {
			// reverse the key/value of snap.Recents to get recentsMap
			recentsMap := make(map[common.Address]uint64, len(snap.Recents))
			bound := uint64(0)
			if n, limit := header.Number.Uint64(), uint64(len(validators)/2+1); n > limit {
				bound = n - limit
			}
			for seen, recent := range snap.Recents {
				if seen <= bound {
					continue
				}
				recentsMap[recent] = seen
			}

			// The backOffTime does not matter when a validator has signed recently.
			if _, ok := recentsMap[val]; ok {
				return 0
			}

			inTurnAddr := validators[(snap.Number+1)%uint64(len(validators))]
			if _, ok := recentsMap[inTurnAddr]; ok {
				log.Debug("in turn validator has recently signed, skip initialBackOffTime",
					"inTurnAddr", inTurnAddr)
				delay = 0
			}

			// Exclude the recently signed validators
			temp := make([]common.Address, 0, len(validators))
			for _, addr := range validators {
				if _, ok := recentsMap[addr]; ok {
					continue
				}
				temp = append(temp, addr)
			}
			validators = temp
		}

		// get the index of current validator and its shuffled backoff time.
		idx := -1
		for index, itemAddr := range validators {
			if val == itemAddr {
				idx = index
			}
		}
		if idx < 0 {
			log.Debug("The validator is not authorized", "addr", val)
			return 0
		}

		s := rand.NewSource(int64(snap.Number))
		r := rand.New(s)
		n := len(validators)
		backOffSteps := make([]uint64, 0, n)

		for i := uint64(0); i < uint64(n); i++ {
			backOffSteps = append(backOffSteps, i)
		}

		r.Shuffle(n, func(i, j int) {
			backOffSteps[i], backOffSteps[j] = backOffSteps[j], backOffSteps[i]
		})

		delay += backOffSteps[idx] * wiggleTime
		return delay
	}
}

func (s *Satoshi) GetJustifiedNumberAndHash(chain consensus.ChainHeaderReader, header []*types.Header) (uint64, common.Hash, error) {
	return 0, common.Hash{}, errors.New("GetJustifiedNumberAndHash not implemented at satoshi")
}

func (s *Satoshi) GetFinalizedHeader(chain consensus.ChainHeaderReader, header *types.Header) *types.Header {
	return nil
}

func (s *Satoshi) VerifyVote(chain consensus.ChainHeaderReader, vote *types.VoteEnvelope) error {
	return errors.New("VerifyVote not implemented at satoshi")
}

func (s *Satoshi) IsActiveValidatorAt(chain consensus.ChainHeaderReader, header *types.Header, checkVoteKeyFn func(bLSPublicKey *types.BLSPublicKey) bool) bool {
	return false
}

// chain context
type chainContext struct {
	Chain     consensus.ChainHeaderReader
	satoshi   consensus.Engine
	feemarket *feemarket.FeeMarket
}

func (c chainContext) Engine() consensus.Engine {
	return c.satoshi
}

func (c chainContext) GetHeader(hash common.Hash, number uint64) *types.Header {
	return c.Chain.GetHeader(hash, number)
}

func (c chainContext) FeeMarket() *feemarket.FeeMarket {
	return c.feemarket
}

// apply message
func applyMessage(
	msg *core.Message,
	evm *vm.EVM,
	state vm.StateDB,
	header *types.Header,
	chainConfig *params.ChainConfig,
	chainContext core.ChainContext,
) (uint64, error) {
	// Increment the nonce for the next transaction
	state.SetNonce(msg.From, state.GetNonce(msg.From)+1, tracing.NonceChangeEoACall)

	ret, returnGas, err := evm.Call(
		vm.AccountRef(msg.From),
		*msg.To,
		msg.Data,
		msg.GasLimit,
		uint256.MustFromBig(msg.Value),
	)
	if err != nil {
		log.Error("apply message failed", "msg", string(ret), "err", err)
	}
	return msg.GasLimit - returnGas, err
}

// proposalKey build a key which is a combination of the block number and the proposer address.
func proposalKey(header types.Header) string {
	return header.ParentHash.String() + header.Coinbase.String()
}
