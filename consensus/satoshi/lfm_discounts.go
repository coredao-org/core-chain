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

// LFMDiscountConfigProvider is a provider for the loading LFM discount configs
// from the system contract
type LFMDiscountConfigProvider struct {
	ethAPI *ethapi.BlockChainAPI
	abi    abi.ABI

	discountConfigsReloadOnNextBlock atomic.Bool // Flag to indicate if the discount configs need to be reloaded on next block

	eoaToEoaDiscount *big.Int
	discountConfigs  map[common.Address]types.LFMDiscountConfig
	// TODO: do we need to add LRUCache here?
	// discountConfigs *lru.Cache[common.Address, LFMDiscountConfig]

	configsBlockNumber uint64 // Block number of the last discount configs reload

	lock sync.RWMutex // Protects the config fields

	// TODO: do we want to reload at various intervals as well?
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

	// Load the discount configs to warm up the in memory cache
	// err = provider.loadDiscountConfigs()
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to load the discount configs: %w", err)
	// }

	return provider, nil
}

// OnBlockStart is called when a new block is started
func (p *LFMDiscountConfigProvider) OnBlockStart(blockNumber uint64) {
	fmt.Println("lmf.OnBlockStart discountConfigsReloadOnNextBlock", p.discountConfigsReloadOnNextBlock.Load())
	if !p.discountConfigsReloadOnNextBlock.Load() {
		return
	}
	fmt.Println("lmf.OnBlockStart discountConfigsReloadOnNextBlock1", blockNumber, "configsBlockNumber", p.configsBlockNumber)
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

	fmt.Println("loadDiscountConfigs.rpcBlockNumber", blockNumber, rpc.BlockNumber(blockNumber))
	rpcBlockNumber := rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(blockNumber))
	fmt.Println("loadDiscountConfigs.rpcBlockNumber1", rpcBlockNumber)

	// method := "discountConfigs"
	method := "getAllAvailableDiscountConfigs"

	// Add timeout of 5 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get packed data
	data, err := p.abi.Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for getAllAvailableDiscountConfigs", "error", err)
		return err
	}

	fmt.Println("start fetch")

	// Call the system contract
	msgData := (hexutil.Bytes)(data)
	toAddress := common.HexToAddress(systemcontracts.LFMDiscountContract)
	gas := (hexutil.Uint64)(uint64(math.MaxUint64 / 2))
	result, err := p.ethAPI.Call(ctx, ethapi.TransactionArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, rpcBlockNumber, nil, nil)

	fmt.Println("end fetch")
	if err != nil {
		log.Error("Failed to fetch discount contract configs", "error", err)
		return err
	}

	// TODO: use LFMDiscountConfig
	var configs []struct {
		Rewards               []types.LFMDiscountReward
		DiscountRate          *big.Int
		UserDiscountRate      *big.Int
		IsActive              bool
		Timestamp             *big.Int
		DiscountAddress       common.Address
		MinimumValidatorShare *big.Int
		IsEOADiscount         bool
	}

	err = p.abi.UnpackIntoInterface(&configs, method, result)
	if err != nil {
		log.Error("Failed to unpack discount contract configs", "error", err)
		return err
	}

	newDiscountConfigs := make(map[common.Address]types.LFMDiscountConfig)
	eoaToEoaDiscount := big.NewInt(0)

	for _, config := range configs {
		if config.IsEOADiscount {
			eoaToEoaDiscount = config.DiscountRate
			fmt.Println("eoaToEoaDiscount", config)
			continue
		}

		discountConfig := types.LFMDiscountConfig{
			Rewards:               config.Rewards,
			DiscountRate:          config.DiscountRate,
			UserDiscountRate:      config.UserDiscountRate,
			IsActive:              config.IsActive,
			Timestamp:             config.Timestamp,
			DiscountAddress:       config.DiscountAddress,
			MinimumValidatorShare: config.MinimumValidatorShare,
		}

		fmt.Println("incoming Discount", config)
		if !isValidConfig(discountConfig) {
			log.Debug("Invalid LFM discount config", "address", discountConfig.DiscountAddress, "config", discountConfig)
			continue
		}

		newDiscountConfigs[discountConfig.DiscountAddress] = discountConfig
	}

	// Update the configs
	p.lock.Lock()
	p.discountConfigs = newDiscountConfigs
	p.eoaToEoaDiscount = eoaToEoaDiscount
	p.configsBlockNumber = blockNumber
	p.lock.Unlock()

	// TODO: remove this
	log.Info("Loaded LFM discount configs", "count", len(newDiscountConfigs), newDiscountConfigs)
	for addr, config := range p.discountConfigs {
		log.Info("LFM discount config", "address", addr, "config", config)
	}

	return nil
}

func (p *LFMDiscountConfigProvider) GetEOAToEOADiscount() *big.Int {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.eoaToEoaDiscount
}

func (p *LFMDiscountConfigProvider) GetDiscountConfigByAddress(address common.Address) (config types.LFMDiscountConfig, ok bool) {
	// // TODO: REMOVE THIS AS IT IS TEMP USED TO ALWAYS RELOAD THE CONFIGS
	// if err := p.loadDiscountConfigs(); err != nil {
	// 	log.Debug("Failed to reload discount configs", "error", err)
	// 	return types.LFMDiscountConfig{}, false
	// }

	// if len(p.discountConfigs) == 0 {
	// 	if err := p.loadDiscountConfigs(); err != nil {
	// 		log.Debug("Failed to reload discount configs", "error", err)
	// 		return types.LFMDiscountConfig{}, false
	// 	}
	// }

	fmt.Println("lmf.GetDiscountConfigByAddress discountConfigsReloadOnNextBlock", p.discountConfigsReloadOnNextBlock.Load())

	fmt.Println("lmf.GetDiscountConfigByAddress", address)
	p.lock.RLock()
	fmt.Println("lmf.GetDiscountConfigByAddress1", p.discountConfigs)
	config, ok = p.discountConfigs[address]
	p.lock.RUnlock()
	fmt.Println("lmf.GetDiscountConfigByAddres2", config, ok)
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

		if reward.RewardPercentage.Cmp(big.NewInt(0)) <= 0 || reward.RewardPercentage.Cmp(big.NewInt(10000)) > 0 {
			return false
		}

		totalRewardPercentage.Add(totalRewardPercentage, reward.RewardPercentage)
	}

	// Verify that DiscountRate = totalRewardPercentage + userDiscountRate
	discountPercentage := big.NewInt(0).Add(totalRewardPercentage, config.UserDiscountRate)
	if discountPercentage.Cmp(config.DiscountRate) != 0 {
		return false
	}

	// TODO: add this check
	// totalDiscount := big.NewInt(0).Add(config.DiscountRate, config.MinimumValidatorShare)
	// if config.MinimumValidatorShare.Cmp(big.NewInt(0)) <= 0 || totalDiscount.Cmp(big.NewInt(10000)) > 0 {
	// 	return false
	// }

	return true
}
