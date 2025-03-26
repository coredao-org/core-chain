package types

import (
	"fmt"

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
func (c FeeMarketConfig) IsValidConfig(denominator, maxGas, maxEvents, maxRewards uint64) bool {
	if denominator == 0 || maxGas == 0 || maxEvents == 0 || maxRewards == 0 {
		fmt.Println("0")
		return false
	}

	if !c.IsActive {
		fmt.Println("1")
		return false
	}

	if c.ConfigAddress == (common.Address{}) {
		fmt.Println("2")
		return false
	}

	if c.Events == nil || len(c.Events) > int(maxEvents) {
		fmt.Println("3")
		return false
	}

	for _, event := range c.Events {
		if event.Gas == 0 || event.Gas > maxGas {
			fmt.Println("event:", event.Gas, "maxGas:", maxGas)
			fmt.Println("4")
			return false
		}

		if event.EventSignature == (common.Hash{}) {
			fmt.Println("5")
			return false
		}

		if event.Rewards == nil || len(event.Rewards) > int(maxRewards) {
			fmt.Println("6")
			return false
		}

		totalRewardPercentage := uint64(0)
		for _, reward := range event.Rewards {
			if reward.RewardAddress == (common.Address{}) {
				fmt.Println("7")
				return false
			}

			if reward.RewardPercentage == 0 || reward.RewardPercentage > denominator {
				fmt.Println("8")
				return false
			}

			totalRewardPercentage += reward.RewardPercentage
		}

		if totalRewardPercentage != denominator {
			fmt.Println("9")
			return false
		}
	}

	// on a later version, we will handle function signatures as well
	return true
}
