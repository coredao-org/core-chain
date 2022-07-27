package systemcontracts

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/params"
)

type UpgradeConfig struct {
	BeforeUpgrade upgradeHook
	AfterUpgrade  upgradeHook
	ContractAddr  common.Address
	CommitUrl     string
	Code          string
}

type Upgrade struct {
	UpgradeName string
	Configs     []*UpgradeConfig
}

type upgradeHook func(blockNumber *big.Int, contractAddr common.Address, statedb *state.StateDB) error

const ()

var (
	GenesisHash common.Hash
)

func init() {
}

func UpgradeBuildInSystemContract(config *params.ChainConfig, blockNumber *big.Int, statedb *state.StateDB) {
	/*
		apply system upgrades
	*/
}

// func applySystemContractUpgrade(upgrade *Upgrade, blockNumber *big.Int, statedb *state.StateDB, logger log.Logger) {
// 	if upgrade == nil {
// 		logger.Info("Empty upgrade config", "height", blockNumber.String())
// 		return
// 	}

// 	logger.Info(fmt.Sprintf("Apply upgrade %s at height %d", upgrade.UpgradeName, blockNumber.Int64()))
// 	for _, cfg := range upgrade.Configs {
// 		logger.Info(fmt.Sprintf("Upgrade contract %s to commit %s", cfg.ContractAddr.String(), cfg.CommitUrl))

// 		if cfg.BeforeUpgrade != nil {
// 			err := cfg.BeforeUpgrade(blockNumber, cfg.ContractAddr, statedb)
// 			if err != nil {
// 				panic(fmt.Errorf("contract address: %s, execute beforeUpgrade error: %s", cfg.ContractAddr.String(), err.Error()))
// 			}
// 		}

// 		newContractCode, err := hex.DecodeString(cfg.Code)
// 		if err != nil {
// 			panic(fmt.Errorf("failed to decode new contract code: %s", err.Error()))
// 		}
// 		statedb.SetCode(cfg.ContractAddr, newContractCode)

// 		if cfg.AfterUpgrade != nil {
// 			err := cfg.AfterUpgrade(blockNumber, cfg.ContractAddr, statedb)
// 			if err != nil {
// 				panic(fmt.Errorf("contract address: %s, execute afterUpgrade error: %s", cfg.ContractAddr.String(), err.Error()))
// 			}
// 		}
// 	}
// }
