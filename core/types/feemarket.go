package types

import (
	"errors"
	"math"

	"github.com/ethereum/go-ethereum/common"
)

// FeeMarketConstants represents the constants of the fee market contract
type FeeMarketConstants struct {
	MaxRewards   uint8
	MaxEvents    uint8
	MaxFunctions uint8
	MaxGas       uint32
}

// FeeMarketReward represents a reward address and percentage
type FeeMarketReward struct {
	RewardAddress    common.Address
	RewardPercentage uint64
}

// FeeMarketEvent represents an event and its associated rewards and gas
type FeeMarketEvent struct {
	EventSignature common.Hash
	Gas            uint64
	Rewards        []FeeMarketReward
}

// FeeMarketFunctionSignatures represents a function signature and its associated rewards and gas
type FeeMarketFunctionSignature struct {
	FunctionSignature common.Hash
	Gas               uint64
	Rewards           []FeeMarketReward
}

// FeeMarketConfig represents a fee monetization configuration
type FeeMarketConfig struct {
	ConfigAddress      common.Address               // The address this config applies to
	IsActive           bool                         // Whether this config is active
	Events             []FeeMarketEvent             // The events this config applies to
	FunctionSignatures []FeeMarketFunctionSignature // The function signatures this config applies to
}

// IsValidConfig checks if a config is valid
func (c FeeMarketConfig) IsValidConfig(constants FeeMarketConstants, denominator uint64) (valid bool, err error) {
	if constants.MaxGas == 0 || constants.MaxEvents == 0 || constants.MaxRewards == 0 {
		return false, errors.New("invalid config constants")
	}

	if !c.IsActive {
		return false, errors.New("config is not active")
	}

	if c.ConfigAddress == (common.Address{}) {
		return false, errors.New("config address is not set")
	}

	if c.Events == nil || len(c.Events) > int(constants.MaxEvents) {
		return false, errors.New("invalid events length")
	}

	for _, event := range c.Events {
		if event.Gas == 0 || event.Gas > math.MaxUint32 || event.Gas > uint64(constants.MaxGas) {
			return false, errors.New("invalid event gas")
		}

		if event.EventSignature == (common.Hash{}) {
			return false, errors.New("invalid event signature")
		}

		if len(event.Rewards) == 0 || len(event.Rewards) > int(constants.MaxRewards) {
			return false, errors.New("invalid rewards length")
		}

		totalRewardPercentage := uint64(0)
		for _, reward := range event.Rewards {
			if reward.RewardAddress == (common.Address{}) {
				return false, errors.New("invalid reward address")
			}

			if reward.RewardPercentage == 0 || reward.RewardPercentage > denominator {
				return false, errors.New("invalid reward percentage")
			}

			totalRewardPercentage += reward.RewardPercentage
		}

		if totalRewardPercentage != denominator {
			return false, errors.New("invalid total rewards percentage")
		}
	}

	// on a later version, we will handle function signatures as well
	return true, nil
}
