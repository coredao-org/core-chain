package systemcontracts

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/systemcontracts/athena"
	"github.com/ethereum/go-ethereum/core/systemcontracts/demeter"
	hashPower "github.com/ethereum/go-ethereum/core/systemcontracts/hash_power"
	"github.com/ethereum/go-ethereum/core/systemcontracts/hera"
	"github.com/ethereum/go-ethereum/core/systemcontracts/luban"
	"github.com/ethereum/go-ethereum/core/systemcontracts/poseidon"
	"github.com/ethereum/go-ethereum/core/systemcontracts/zeus"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
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

type upgradeHook func(blockNumber *big.Int, contractAddr common.Address, statedb vm.StateDB) error

const (
	mainNet    = "Mainnet"
	buffaloNet = "Buffalo"
	pigeonNet  = "Pigeon"
	defaultNet = "Default"
)

const ()

var (
	GenesisHash common.Hash
	//upgrade config
	hashPowerUpgrade = make(map[string]*Upgrade)

	zeusUpgrade = make(map[string]*Upgrade)

	heraUpgrade = make(map[string]*Upgrade)

	poseidonUpgrade = make(map[string]*Upgrade)

	demeterUpgrade = make(map[string]*Upgrade)

	athenaUpgrade = make(map[string]*Upgrade)

	lubanUpgrade = make(map[string]*Upgrade)

	// TODO(cz): Chech which ones to keep below
	// haberFixUpgrade = make(map[string]*Upgrade)

	// bohrUpgrade = make(map[string]*Upgrade)

	// pascalUpgrade = make(map[string]*Upgrade)

	// lorentzUpgrade = make(map[string]*Upgrade)
)

func init() {
	hashPowerUpgrade[buffaloNet] = &Upgrade{
		UpgradeName: "hashPower",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SlashContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloSlashContract,
			},
			{
				ContractAddr: common.HexToAddress(SystemRewardContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloSystemRewardContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(RelayerHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloRelayerHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(BurnContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloBurnContract,
			},
			{
				ContractAddr: common.HexToAddress(FoundationContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/52afb1322a69f8e695ab35227bcdff6c65ee752a",
				Code:         hashPower.BuffaloFoundationContract,
			},
		},
	}
	zeusUpgrade[buffaloNet] = &Upgrade{
		UpgradeName: "zeus",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(FoundationContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/4c8ca83979a34333ee8734fd57ab84f309539b5b",
				Code:         zeus.BuffaloFoundationContract,
			},
		},
	}
	zeusUpgrade[mainNet] = &Upgrade{
		UpgradeName: "zeus",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(FoundationContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/d545ede09f6af0b2c2451f739234e582f0aeeb2b",
				Code:         zeus.MainnetFoundationContract,
			},
		},
	}
	heraUpgrade[buffaloNet] = &Upgrade{
		UpgradeName: "hera",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/11af2de8a60dec511b6752f6f42a86f372c32b5f",
				Code:         hera.BuffaloLightClientContract,
			},
		},
	}
	heraUpgrade[mainNet] = &Upgrade{
		UpgradeName: "hera",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/6b8aad810e5e023352bedadc022eed9a280b6367",
				Code:         hera.MainnetLightClientContract,
			},
		},
	}
	poseidonUpgrade[buffaloNet] = &Upgrade{
		UpgradeName: "poseidon",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/f95ba12cc2baf8f4c13e2dd2c4278f33a0081aed",
				Code:         poseidon.BuffaloValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/f95ba12cc2baf8f4c13e2dd2c4278f33a0081aed",
				Code:         poseidon.BuffaloLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/f95ba12cc2baf8f4c13e2dd2c4278f33a0081aed",
				Code:         poseidon.BuffaloCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/f95ba12cc2baf8f4c13e2dd2c4278f33a0081aed",
				Code:         poseidon.BuffaloPledgeCandidateContract,
			},
		},
	}
	poseidonUpgrade[mainNet] = &Upgrade{
		UpgradeName: "poseidon",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/5e846bfd00de9d59ab32005e0bb7916615d8c764",
				Code:         poseidon.MainnetValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/5e846bfd00de9d59ab32005e0bb7916615d8c764",
				Code:         poseidon.MainnetLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/5e846bfd00de9d59ab32005e0bb7916615d8c764",
				Code:         poseidon.MainnetCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/5e846bfd00de9d59ab32005e0bb7916615d8c764",
				Code:         poseidon.MainnetPledgeCandidateContract,
			},
		},
	}
	demeterUpgrade[buffaloNet] = &Upgrade{
		UpgradeName: "demeter",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SlashContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloSlashContract,
			},
			{
				ContractAddr: common.HexToAddress(SystemRewardContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloSystemRewardContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(RelayerHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloRelayerHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(BurnContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloBurnContract,
			},
			{
				ContractAddr: common.HexToAddress(FoundationContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloFoundationContract,
			},
			{
				ContractAddr: common.HexToAddress(StakeHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloStakeHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CoreAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloCoreAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(HashAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloHashAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloBTCAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloBTCStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloBTCLSTStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTTokenContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/8c4176806d6646c5ce37a33ac3dd067dae3294f7",
				Code:         demeter.BuffaloBTCLSTTokenContract,
			},
		},
	}
	demeterUpgrade[mainNet] = &Upgrade{
		UpgradeName: "demeter",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SlashContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetSlashContract,
			},
			{
				ContractAddr: common.HexToAddress(SystemRewardContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetSystemRewardContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(RelayerHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetRelayerHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(BurnContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetBurnContract,
			},
			{
				ContractAddr: common.HexToAddress(FoundationContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetFoundationContract,
			},
			{
				ContractAddr: common.HexToAddress(StakeHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetStakeHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CoreAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetCoreAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(HashAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetHashAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetBTCAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetBTCStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetBTCLSTStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTTokenContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/925e416b42cb97ffba3b1bad4394d7e609452e10",
				Code:         demeter.MainnetBTCLSTTokenContract,
			},
		},
	}
	athenaUpgrade[pigeonNet] = &Upgrade{
		UpgradeName: "athena",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SystemRewardContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonSystemRewardContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(BurnContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonBurnContract,
			},
			{
				ContractAddr: common.HexToAddress(StakeHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonStakeHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CoreAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonCoreAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(HashAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonHashAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonBTCAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonBTCStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/76bb9aec72556db2646d449fca9b8832d5829ec1",
				Code:         athena.PigeonBTCLSTStakeContract,
			},
		},
	}
	athenaUpgrade[mainNet] = &Upgrade{
		UpgradeName: "athena",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SystemRewardContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetSystemRewardContract,
			},
			{
				ContractAddr: common.HexToAddress(LightClientContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetLightClientContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetCandidateHubContract,
			},
			{
				ContractAddr: common.HexToAddress(GovHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetGovHubContract,
			},
			{
				ContractAddr: common.HexToAddress(PledgeCandidateContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetPledgeCandidateContract,
			},
			{
				ContractAddr: common.HexToAddress(BurnContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetBurnContract,
			},
			{
				ContractAddr: common.HexToAddress(StakeHubContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetStakeHubContract,
			},
			{
				ContractAddr: common.HexToAddress(CoreAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetCoreAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(HashAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetHashAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCAgentContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetBTCAgentContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetBTCStakeContract,
			},
			{
				ContractAddr: common.HexToAddress(BTCLSTStakeContract),
				CommitUrl:    "https://github.com/coredao-org/core-genesis-contract/commit/48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
				Code:         athena.MainnetBTCLSTStakeContract,
			},
		},
	}
	lubanUpgrade[defaultNet] = &Upgrade{
		UpgradeName: "luban",
		Configs: []*UpgradeConfig{
			{
				ContractAddr: common.HexToAddress(ValidatorContract),
				CommitUrl:    "https://github.com/bnb-chain/bsc-genesis-contract/commit/b57652fbdd87e6436dd1685663b87b6036bdd762",
				Code:         luban.DefaultValidatorContract,
			},
			{
				ContractAddr: common.HexToAddress(SlashContract),
				CommitUrl:    "https://github.com/bnb-chain/bsc-genesis-contract/commit/b57652fbdd87e6436dd1685663b87b6036bdd762",
				Code:         luban.DefaultSlashContract,
			},
			{
				ContractAddr: common.HexToAddress(CandidateHubContract),
				CommitUrl:    "https://github.com/bnb-chain/bsc-genesis-contract/commit/b57652fbdd87e6436dd1685663b87b6036bdd762",
				Code:         luban.DefaultCandidateHubContract,
			},
		},
	}
}

// TODO(cz): check this as it is being called with atBlockBegin from more places
func TryUpdateBuildInSystemContract(config *params.ChainConfig, blockNumber *big.Int, lastBlockTime uint64, blockTime uint64, statedb vm.StateDB, atBlockBegin bool) {
	if atBlockBegin {
		if !config.IsHermes(blockNumber, lastBlockTime) {
			upgradeBuildInSystemContract(config, blockNumber, lastBlockTime, blockTime, statedb)
		}
		// TODO(cz): BEP-440 EIP-2935 https://github.com/bnb-chain/bsc/pull/2721
		// HistoryStorageAddress is a special system contract in bsc, which can't be upgraded
		if config.IsOnHermes(blockNumber, lastBlockTime, blockTime) {
			statedb.SetCode(params.HistoryStorageAddress, params.HistoryStorageCode)
			statedb.SetNonce(params.HistoryStorageAddress, 1, tracing.NonceChangeNewContract)
			log.Info("Set code for HistoryStorageAddress", "blockNumber", blockNumber.Int64(), "blockTime", blockTime)
		}
	} else {
		if config.IsHermes(blockNumber, lastBlockTime) {
			upgradeBuildInSystemContract(config, blockNumber, lastBlockTime, blockTime, statedb)
		}
	}
}

func upgradeBuildInSystemContract(config *params.ChainConfig, blockNumber *big.Int, lastBlockTime uint64, blockTime uint64, statedb vm.StateDB) {
	if config == nil || blockNumber == nil || statedb == nil || reflect.ValueOf(statedb).IsNil() {
		return
	}

	var network string
	switch GenesisHash {
	/* Add mainnet genesis hash */
	case params.CoreGenesisHash:
		network = mainNet
	case params.BuffaloGenesisHash:
		network = buffaloNet
	case params.PigeonGenesisHash:
		network = pigeonNet
	default:
		network = defaultNet
	}

	logger := log.New("system-contract-upgrade", network)
	if config.IsOnHashPower(blockNumber) {
		applySystemContractUpgrade(hashPowerUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnZeus(blockNumber) {
		applySystemContractUpgrade(zeusUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnHera(blockNumber) {
		applySystemContractUpgrade(heraUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnPoseidon(blockNumber) {
		applySystemContractUpgrade(poseidonUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnDemeter(blockNumber, lastBlockTime, blockTime) {
		applySystemContractUpgrade(demeterUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnAthena(blockNumber, lastBlockTime, blockTime) {
		applySystemContractUpgrade(athenaUpgrade[network], blockNumber, statedb, logger)
	}
	if config.IsOnLuban(blockNumber, lastBlockTime, blockTime) {
		applySystemContractUpgrade(lubanUpgrade[network], blockNumber, statedb, logger)
	}
	/*
		apply other upgrades
	*/
}

func applySystemContractUpgrade(upgrade *Upgrade, blockNumber *big.Int, statedb vm.StateDB, logger log.Logger) {
	if upgrade == nil {
		logger.Info("Empty upgrade config", "height", blockNumber.String())
		return
	}

	logger.Info(fmt.Sprintf("Apply upgrade %s at height %d", upgrade.UpgradeName, blockNumber.Int64()))
	for _, cfg := range upgrade.Configs {
		logger.Info(fmt.Sprintf("Upgrade contract %s to commit %s", cfg.ContractAddr.String(), cfg.CommitUrl))

		if cfg.BeforeUpgrade != nil {
			err := cfg.BeforeUpgrade(blockNumber, cfg.ContractAddr, statedb)
			if err != nil {
				panic(fmt.Errorf("contract address: %s, execute beforeUpgrade error: %s", cfg.ContractAddr.String(), err.Error()))
			}
		}

		newContractCode, err := hex.DecodeString(strings.TrimSpace(cfg.Code))
		if err != nil {
			panic(fmt.Errorf("failed to decode new contract code: %s", err.Error()))
		}
		statedb.SetCode(cfg.ContractAddr, newContractCode)

		if cfg.AfterUpgrade != nil {
			err := cfg.AfterUpgrade(blockNumber, cfg.ContractAddr, statedb)
			if err != nil {
				panic(fmt.Errorf("contract address: %s, execute afterUpgrade error: %s", cfg.ContractAddr.String(), err.Error()))
			}
		}
	}
}
