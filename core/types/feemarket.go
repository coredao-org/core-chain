package types

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

// FeeMarketReward represents a reward address and percentage
type FeeMarketReward struct {
	RewardAddress    common.Address
	RewardPercentage *uint256.Int
}

// FeeMarketEvent represents an event and its associated rewards and gas
type FeeMarketEvent struct {
	Rewards        []FeeMarketReward
	EventSignature common.Hash
	Gas            *uint256.Int
}

// FeeMarketFunctionSignatures represents a function signature and its associated rewards and gas
type FeeMarketFunctionSignature struct {
	Rewards           []FeeMarketReward
	FunctionSignature common.Hash
	Gas               *uint256.Int
}

// FeeMarketConfig represents a fee monetization configuration
type FeeMarketConfig struct {
	Events             []FeeMarketEvent             // The events this config applies to
	FunctionSignatures []FeeMarketFunctionSignature // The function signatures this config applies to
	ConfigAddress      common.Address               // The address this config applies to
	IsActive           bool                         // Whether this config is active
}

// IsValidConfig checks if a config is valid
func (c FeeMarketConfig) IsValidConfig(denominator *uint256.Int) bool {
	return true
	// if denominator == nil && denominator.Sign() <= 0 {
	// 	return false
	// }

	// if !c.IsActive {
	// 	return false
	// }

	// if c.ConfigAddress == (common.Address{}) {
	// 	return false
	// }

	// if c.ConfigRate == nil || !isValidConfigRate(c.ConfigRate, denominator) {
	// 	return false
	// }

	// if c.UserConfigRate == nil || !isValidConfigRate(c.UserConfigRate, denominator) {
	// 	return false
	// }

	// totalRewardPercentage := uint256.NewInt(0)
	// for _, reward := range c.Rewards {
	// 	if reward.RewardAddress == (common.Address{}) {
	// 		return false
	// 	}

	// 	if reward.RewardPercentage == nil || !isValidConfigRate(reward.RewardPercentage, denominator) {
	// 		return false
	// 	}

	// 	totalRewardPercentage.Add(totalRewardPercentage, reward.RewardPercentage)
	// }

	// // Verify that configRate = totalRewardPercentage + userConfigRate
	// totalConfigRate := uint256.NewInt(0).Add(totalRewardPercentage, c.UserConfigRate)
	// return c.ConfigRate.Cmp(totalConfigRate) == 0
}

// isValidConfigRate validates the rate to be positive and below the denominator.
func isValidConfigRate(rate *uint256.Int, denominator *uint256.Int) bool {
	return rate != nil && rate.Sign() >= 0 && rate.Cmp(denominator) <= 0
}
