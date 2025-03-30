package types

import (
	"errors"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
)

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
func (c FeeMarketConfig) IsValidConfig(denominator, maxGas, maxEvents, maxRewards uint64) (valid bool, err error) {
	if denominator == 0 || maxGas == 0 || maxEvents == 0 || maxRewards == 0 {
		return false, errors.New("invalid config constants")
	}

	if !c.IsActive {
		return false, errors.New("config is not active")
	}

	if c.ConfigAddress == (common.Address{}) {
		return false, errors.New("config address is not set")
	}

	if c.Events == nil || len(c.Events) > int(maxEvents) {
		return false, errors.New("invalid events length")
	}

	for _, event := range c.Events {
		if event.Gas == 0 || event.Gas > maxGas {
			return false, errors.New("invalid gas for event " + event.EventSignature.String())
		}

		if event.EventSignature == (common.Hash{}) {
			return false, errors.New("invalid event signature")
		}

		if len(event.Rewards) == 0 || len(event.Rewards) > int(maxRewards) {
			return false, errors.New("invalid rewards length")
		}

		totalRewardPercentage := uint64(0)
		for i, reward := range event.Rewards {
			if reward.RewardAddress == (common.Address{}) {
				return false, errors.New("invalid reward address for reward at index " + strconv.Itoa(i))
			}

			if reward.RewardPercentage == 0 || reward.RewardPercentage > denominator {
				return false, errors.New("invalid reward percentage for reward at index " + strconv.Itoa(i))
			}

			totalRewardPercentage += reward.RewardPercentage
		}

		if totalRewardPercentage != denominator {
			return false, errors.New("invalid total reward percentage, should be 100%")
		}
	}

	// on a later version, we will handle function signatures as well
	return true, nil
}
