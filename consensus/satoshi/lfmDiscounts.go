package satoshi

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/systemcontracts"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

// LFMDiscountConfigProvider is a provider for the loading LFM discount configs
// from the system contract
type LFMDiscountConfigProvider struct {
	ethAPI *ethapi.BlockChainAPI
	abi    abi.ABI

	discountConfigsReloadOnNextBlock atomic.Bool // Flag to indicate if the discount configs need to be reloaded on next block

	eoaToEoaDiscount *big.Int
	discountConfigs  map[common.Address]types.LFMDiscountConfig

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
	p.lock.Lock()
	defer p.lock.Unlock()

	rpcBlockNumber := rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(blockNumber))
	method := "getAllAvailableDiscountConfigs"

	ctx, cancel := context.WithCancel(context.Background())
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
		return err
	}

	// TODO: use LFMDiscountConfig
	var configs []struct {
		Rewards          []types.LFMDiscountReward
		DiscountRate     *big.Int
		UserDiscountRate *big.Int
		IsActive         bool
		Timestamp        *big.Int
		DiscountAddress  common.Address
	}

	err = p.abi.UnpackIntoInterface(&configs, method, result)
	if err != nil {
		log.Error("Failed to unpack contract result", "error", err)
		return err
	}

	newDiscountConfigs := make(map[common.Address]types.LFMDiscountConfig)

	for _, config := range configs {
		conf := types.LFMDiscountConfig{
			Rewards:          config.Rewards,
			DiscountRate:     config.DiscountRate,
			UserDiscountRate: config.UserDiscountRate,
			IsActive:         config.IsActive,
			Timestamp:        config.Timestamp,
			DiscountAddress:  config.DiscountAddress,
		}

		if !isValidConfig(conf) {
			log.Debug("Invalid LFM discount config", "address", conf.DiscountAddress, "config", conf)
			continue
		}

		newDiscountConfigs[conf.DiscountAddress] = conf
	}

	// Update the configs
	p.discountConfigs = newDiscountConfigs
	p.configsBlockNumber = blockNumber

	return nil
}

func (p *LFMDiscountConfigProvider) GetEOAToEOADiscount() *big.Int {
	return p.eoaToEoaDiscount
}

func (p *LFMDiscountConfigProvider) GetDiscountConfigByAddress(address common.Address) (config types.LFMDiscountConfig, ok bool) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	config, ok = p.discountConfigs[address]
	return config, ok
}

// isValidConfig checks if a given LFM discount config is valid
func isValidConfig(config types.LFMDiscountConfig) bool {
	if !config.IsActive {
		return false
	}

	if config.DiscountAddress == (common.Address{}) {
		return false
	}

	// TODO: check if bigger than max discount rate constant from the contract
	if config.DiscountRate.Cmp(big.NewInt(0)) <= 0 || config.DiscountRate.Cmp(big.NewInt(10000)) > 0 {
		return false
	}

	if config.UserDiscountRate.Cmp(big.NewInt(0)) <= 0 {
		return false
	}

	totalRewardPercentage := big.NewInt(0)
	for _, reward := range config.Rewards {
		if reward.RewardAddress == (common.Address{}) {
			return false
		}

		if reward.RewardPercentage.Cmp(big.NewInt(0)) <= 0 {
			return false
		}

		totalRewardPercentage.Add(totalRewardPercentage, reward.RewardPercentage)
	}

	// Verify that DiscountRate = totalRewardPercentage + userDiscountRate
	discountPercentage := big.NewInt(0).Add(totalRewardPercentage, config.UserDiscountRate)
	if discountPercentage.Cmp(config.DiscountRate) != 0 {
		return false
	}

	return true
}
