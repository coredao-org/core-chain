package feemarket

import (
	"encoding/json"
	"fmt"
	"time"
)

// TODO: DO NOT REVIEW THIS FILE, is a helper for testing

// CacheReport is a JSON-friendly representation of the FeeMarketCache state
type CacheReport struct {
	HeadHash    string                        `json:"headHash"`
	HeadHeight  uint64                        `json:"headHeight"`
	ParentHash  string                        `json:"parentHash"`
	Constants   *ConstantsReport              `json:"constants,omitempty"`
	TempConsts  map[string]*ConstantsReport   `json:"tempConstants,omitempty"`
	Entries     map[string]*ConfigReport      `json:"entries,omitempty"`
	TempEntries map[string]*TempConfigsReport `json:"tempEntries,omitempty"`
}

// ConstantsReport is a JSON-friendly representation of FeeMarketConstants
type ConstantsReport struct {
	BlockNum    uint64    `json:"blockNum"`
	BlockHash   string    `json:"blockHash,omitempty"`
	Modified    time.Time `json:"modified"`
	Denominator uint64    `json:"denominator"`
	MaxRewards  uint64    `json:"maxRewards"`
	MaxGas      uint64    `json:"maxGas"`
	MaxEvents   uint64    `json:"maxEvents"`
	MaxFuncSigs uint64    `json:"maxFunctionSignatures"`
}

// ConfigReport is a JSON-friendly representation of FeeMarketConfig
type ConfigReport struct {
	BlockNum  uint64          `json:"blockNum"`
	BlockHash string          `json:"blockHash,omitempty"`
	Modified  time.Time       `json:"modified"`
	IsActive  bool            `json:"isActive"`
	Events    []EventReport   `json:"events,omitempty"`
	FuncSigs  []FuncSigReport `json:"functionSignatures,omitempty"`
}

// EventReport is a JSON-friendly representation of FeeMarketEvent
type EventReport struct {
	EventSignature string         `json:"eventSignature"`
	Gas            uint64         `json:"gas"`
	Rewards        []RewardReport `json:"rewards,omitempty"`
}

// FuncSigReport is a JSON-friendly representation of FeeMarketFunctionSignature
type FuncSigReport struct {
	FunctionSignature string         `json:"functionSignature"`
	Gas               uint64         `json:"gas"`
	Rewards           []RewardReport `json:"rewards,omitempty"`
}

// RewardReport is a JSON-friendly representation of FeeMarketReward
type RewardReport struct {
	RewardAddress    string `json:"rewardAddress"`
	RewardPercentage uint64 `json:"rewardPercentage"`
}

// TempConfigsReport is a JSON-friendly representation of MiningConfigsCache
type TempConfigsReport struct {
	WorkID  string                   `json:"workID"`
	Entries map[string]*ConfigReport `json:"entries,omitempty"`
}

// ReportJSON returns a JSON representation of the FeeMarketCache state
func (c *FeeMarketCache) ReportJSON() (string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	report := &CacheReport{
		HeadHash:    c.head.Hex(),
		HeadHeight:  c.headHeight,
		ParentHash:  c.parentHash.Hex(),
		TempConsts:  make(map[string]*ConstantsReport),
		Entries:     make(map[string]*ConfigReport),
		TempEntries: make(map[string]*TempConfigsReport),
	}

	// Add constants if they exist
	if c.constants != nil {
		report.Constants = &ConstantsReport{
			BlockNum:    c.constants.blockNum,
			BlockHash:   c.constants.blockHash.Hex(),
			Modified:    c.constants.modified,
			Denominator: c.constants.constants.Denominator,
			MaxRewards:  c.constants.constants.MaxRewards,
			MaxGas:      c.constants.constants.MaxGas,
			MaxEvents:   c.constants.constants.MaxEvents,
			MaxFuncSigs: c.constants.constants.MaxFunctionSignatures,
		}
	}

	// Add temporary constants
	for workID, tempConst := range c.tempConstantsMap {
		workIDStr := fmt.Sprintf("%s:%d:%d", workID.ParentHash.Hex(), workID.Timestamp, workID.AttemptNum)
		if tempConst.entry != nil {
			report.TempConsts[workIDStr] = &ConstantsReport{
				BlockNum:    tempConst.entry.blockNum,
				BlockHash:   tempConst.entry.blockHash.Hex(),
				Modified:    tempConst.entry.modified,
				Denominator: tempConst.entry.constants.Denominator,
				MaxRewards:  tempConst.entry.constants.MaxRewards,
				MaxGas:      tempConst.entry.constants.MaxGas,
				MaxEvents:   tempConst.entry.constants.MaxEvents,
				MaxFuncSigs: tempConst.entry.constants.MaxFunctionSignatures,
			}
		}
	}

	// Add main entries
	for addr, entry := range c.entries {
		report.Entries[addr.Hex()] = createConfigReport(entry)
	}

	// Add temporary entries
	for workID, tempCache := range c.tempEntriesMap {
		workIDStr := fmt.Sprintf("%s:%d:%d", workID.ParentHash.Hex(), workID.Timestamp, workID.AttemptNum)
		tempReport := &TempConfigsReport{
			WorkID:  workIDStr,
			Entries: make(map[string]*ConfigReport),
		}

		for addr, entry := range tempCache.entries {
			tempReport.Entries[addr.Hex()] = createConfigReport(entry)
		}

		report.TempEntries[workIDStr] = tempReport
	}

	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// createConfigReport is a helper function to create a ConfigReport from a ConfigEntry
func createConfigReport(entry *ConfigEntry) *ConfigReport {
	report := &ConfigReport{
		BlockNum:  entry.blockNum,
		BlockHash: entry.blockHash.Hex(),
		Modified:  entry.modified,
		IsActive:  entry.config.IsActive,
	}

	// Convert events
	if len(entry.config.Events) > 0 {
		report.Events = make([]EventReport, len(entry.config.Events))
		for i, event := range entry.config.Events {
			eventReport := EventReport{
				EventSignature: event.EventSignature.Hex(),
				Gas:            event.Gas,
			}

			// Convert rewards
			if len(event.Rewards) > 0 {
				eventReport.Rewards = make([]RewardReport, len(event.Rewards))
				for j, reward := range event.Rewards {
					eventReport.Rewards[j] = RewardReport{
						RewardAddress:    reward.RewardAddress.Hex(),
						RewardPercentage: reward.RewardPercentage,
					}
				}
			}

			report.Events[i] = eventReport
		}
	}

	// Convert function signatures
	if len(entry.config.FunctionSignatures) > 0 {
		report.FuncSigs = make([]FuncSigReport, len(entry.config.FunctionSignatures))
		for i, funcSig := range entry.config.FunctionSignatures {
			funcSigReport := FuncSigReport{
				FunctionSignature: funcSig.FunctionSignature.Hex(),
				Gas:               funcSig.Gas,
			}

			// Convert rewards
			if len(funcSig.Rewards) > 0 {
				funcSigReport.Rewards = make([]RewardReport, len(funcSig.Rewards))
				for j, reward := range funcSig.Rewards {
					funcSigReport.Rewards[j] = RewardReport{
						RewardAddress:    reward.RewardAddress.Hex(),
						RewardPercentage: reward.RewardPercentage,
					}
				}
			}

			report.FuncSigs[i] = funcSigReport
		}
	}

	return report
}
