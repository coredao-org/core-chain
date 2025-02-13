package types

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// LFMDiscountReward is holds the reward address and the reward percentage for the LFM discount
type LFMDiscountReward struct {
	RewardAddress    common.Address `json:"rewardAddress"`
	RewardPercentage *big.Int       `json:"rewardPercentage"`
}

// LFMDiscountConfig is the config for an LFM discount
type LFMDiscountConfig struct {
	Rewards               []LFMDiscountReward `json:"rewards"`
	DiscountRate          *big.Int            `json:"discountRate"`
	UserDiscountRate      *big.Int            `json:"userDiscountRate"`
	IsActive              bool                `json:"isActive"`
	Timestamp             *big.Int            `json:"timestamp"`
	DiscountAddress       common.Address      `json:"discountAddress"`
	MinimumValidatorShare *big.Int            `json:"minimumValidatorShare"`
}
