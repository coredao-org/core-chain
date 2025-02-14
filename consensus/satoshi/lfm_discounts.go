package satoshi

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

var LFM_DISCOUNT_PERCENTAGE_DENOMINATOR = uint64(10000)

// LFMDiscountConfigProvider is a provider for the loading LFM discount configs
// from the system contract
type LFMDiscountConfigProvider struct {
	ethAPI *ethapi.BlockChainAPI
	abi    abi.ABI

	discountConfigsReloadOnNextBlock atomic.Bool // Flag to indicate if the discount configs need to be reloaded on next block

	eoaToEoaDiscount         *big.Int
	eoaMinimumValidatorShare *big.Int

	discountConfigs map[common.Address]types.LFMDiscountConfig

	configsBlockNumber uint64 // Block number of the last discount configs reload

	lock sync.RWMutex // Protects the config fields
}

// NewLFMDiscountConfigProvider creates a new LFM discount config provider,
// which loads the discount configs from the system contract automatically,
// while allowing to invalidate the configs at any time in order to reload the latest ones.
func NewLFMDiscountConfigProvider(
	ethAPI *ethapi.BlockChainAPI,
) (*LFMDiscountConfigProvider, error) {
	lABI, err := abi.JSON(strings.NewReader(lmfDiscountABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse the ABI for the LFM discount config provider: %w", err)
	}

	provider := &LFMDiscountConfigProvider{
		ethAPI:          ethAPI,
		abi:             lABI,
		discountConfigs: make(map[common.Address]types.LFMDiscountConfig),
	}

	// Force load on next block
	provider.discountConfigsReloadOnNextBlock.Store(true)

	return provider, nil
}

// OnBlockStart is called when a new block is started
func (p *LFMDiscountConfigProvider) OnBlockStart(blockNumber uint64) {
	if !p.discountConfigsReloadOnNextBlock.Load() {
		return
	}
	p.discountConfigsReloadOnNextBlock.Store(false)

	p.loadDiscountConfigs(blockNumber)
}

// ReloadOnNextBlock marks the discount configs reload on next block
func (p *LFMDiscountConfigProvider) ReloadOnNextBlock() {
	p.discountConfigsReloadOnNextBlock.Store(true)
}

// loadDiscountConfigs loads the discount configs from the system contract
func (p *LFMDiscountConfigProvider) loadDiscountConfigs(blockNumber uint64) error {
	// Skip loading configs for genesis block
	if blockNumber == 0 {
		return nil
	}

	rpcBlockNumber := rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(blockNumber))

	// method := "discountConfigs"
	method := "getAllDiscountConfigs"

	// Add timeout of 5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get packed data
	data, err := p.abi.Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for getAllAvailableDiscountConfigs", "error", err)
		return err
	}

	// Call the system contract
	msgData := (hexutil.Bytes)(data)
	toAddress := common.HexToAddress(systemcontracts.LFMDiscountContract)
	gas := (hexutil.Uint64)(uint64(math.MaxUint64 / 2))
	result, err := p.ethAPI.Call(ctx, ethapi.TransactionArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, rpcBlockNumber, nil, nil)

	if err != nil {
		log.Error("Failed to fetch discount contract configs", "error", err)
		return err
	}

	var configs []struct {
		DiscountRate          *big.Int
		UserDiscountRate      *big.Int
		IsActive              bool
		Timestamp             *big.Int
		DiscountAddress       common.Address
		MinimumValidatorShare *big.Int
		IsEOADiscount         bool
		Rewards               []types.LFMDiscountReward
	}

	err = p.abi.UnpackIntoInterface(&configs, method, result)
	if err != nil {
		log.Error("Failed to unpack discount contract configs", "error", err)
		return err
	}

	newDiscountConfigs := make(map[common.Address]types.LFMDiscountConfig)
	eoaToEoaDiscount := big.NewInt(0)
	eoaMinimumValidatorShare := big.NewInt(0)

	for _, config := range configs {
		discountConfig := types.LFMDiscountConfig{
			Rewards:               config.Rewards,
			DiscountRate:          config.DiscountRate,
			UserDiscountRate:      config.UserDiscountRate,
			IsActive:              config.IsActive,
			Timestamp:             config.Timestamp,
			DiscountAddress:       config.DiscountAddress,
			MinimumValidatorShare: config.MinimumValidatorShare,
		}

		if !p.isValidConfig(discountConfig, config.IsEOADiscount) {
			log.Info("Invalid LFM discount config", "address", discountConfig.DiscountAddress, "config", discountConfig)
			continue
		}

		if config.IsEOADiscount {
			eoaToEoaDiscount = config.DiscountRate
			eoaMinimumValidatorShare = config.MinimumValidatorShare
			continue
		}

		newDiscountConfigs[discountConfig.DiscountAddress] = discountConfig
	}

	// Update the configs
	p.lock.Lock()
	p.discountConfigs = newDiscountConfigs
	p.eoaToEoaDiscount = eoaToEoaDiscount
	p.eoaMinimumValidatorShare = eoaMinimumValidatorShare
	p.configsBlockNumber = blockNumber
	p.lock.Unlock()


	return nil
}

func (p *LFMDiscountConfigProvider) GetEOAToEOADiscount() (eoaToEoaDiscount *big.Int, eoaMinimumValidatorShare *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.eoaToEoaDiscount, p.eoaMinimumValidatorShare
}

func (p *LFMDiscountConfigProvider) GetDiscountConfigByAddress(address common.Address) (config types.LFMDiscountConfig, ok bool) {
	p.lock.RLock()
	config, ok = p.discountConfigs[address]
	p.lock.RUnlock()
	return config, ok
}

func (p *LFMDiscountConfigProvider) GetDiscountPercentageDenominator() *big.Int {
	return new(big.Int).SetUint64(LFM_DISCOUNT_PERCENTAGE_DENOMINATOR)
}

// IsValidDiscountRate checks if the given rate is valid according to the minimum validator share constraints.
// The rate must be positive and not exceed the maximum allowed percentage (10000 - minimumValidatorShare).
func (p *LFMDiscountConfigProvider) IsValidDiscountRate(rate *big.Int, minimumValidatorShare *big.Int) bool {
	hasValidMinimumValidatorShare := minimumValidatorShare != nil && minimumValidatorShare.Sign() > 0 && minimumValidatorShare.Cmp(new(big.Int).SetUint64(LFM_DISCOUNT_PERCENTAGE_DENOMINATOR)) <= 0

	return hasValidMinimumValidatorShare && rate != nil && rate.Sign() > 0 && rate.Cmp(big.NewInt(0).Sub(new(big.Int).SetUint64(LFM_DISCOUNT_PERCENTAGE_DENOMINATOR), minimumValidatorShare)) <= 0
}

// isValidConfig checks if a given LFM discount config is valid
func (p *LFMDiscountConfigProvider) isValidConfig(config types.LFMDiscountConfig, isEOA bool) bool {
	if !config.IsActive {
		return false
	}

	if config.DiscountAddress == (common.Address{}) && !isEOA {
		return false
	}

	if !p.IsValidDiscountRate(config.DiscountRate, config.MinimumValidatorShare) {
		return false
	}

	totalRewardPercentage := big.NewInt(0)
	for _, reward := range config.Rewards {
		if reward.RewardAddress == (common.Address{}) {
			return false
		}

		if !p.IsValidDiscountRate(reward.RewardPercentage, config.MinimumValidatorShare) {
			return false
		}

		totalRewardPercentage.Add(totalRewardPercentage, reward.RewardPercentage)
	}

	// Verify that discountRate = totalRewardPercentage + userDiscountRate
	totalDiscountRate := big.NewInt(0).Add(totalRewardPercentage, config.UserDiscountRate)
	return config.DiscountRate.Cmp(totalDiscountRate) == 0
}
