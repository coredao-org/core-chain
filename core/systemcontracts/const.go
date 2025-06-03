package systemcontracts

import (
	"maps"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// genesis contracts
	ValidatorContract       = "0x0000000000000000000000000000000000001000"
	SlashContract           = "0x0000000000000000000000000000000000001001"
	SystemRewardContract    = "0x0000000000000000000000000000000000001002"
	LightClientContract     = "0x0000000000000000000000000000000000001003"
	RelayerHubContract      = "0x0000000000000000000000000000000000001004"
	CandidateHubContract    = "0x0000000000000000000000000000000000001005"
	GovHubContract          = "0x0000000000000000000000000000000000001006"
	PledgeCandidateContract = "0x0000000000000000000000000000000000001007"
	BurnContract            = "0x0000000000000000000000000000000000001008"
	FoundationContract      = "0x0000000000000000000000000000000000001009"
	StakeHubContract        = "0x0000000000000000000000000000000000001010"
	CoreAgentContract       = "0x0000000000000000000000000000000000001011"
	HashAgentContract       = "0x0000000000000000000000000000000000001012"
	BTCAgentContract        = "0x0000000000000000000000000000000000001013"
	BTCStakeContract        = "0x0000000000000000000000000000000000001014"
	BTCLSTStakeContract     = "0x0000000000000000000000000000000000001015"
	FeeMarketContract       = "0x0000000000000000000000000000000000001016"
	BTCLSTTokenContract     = "0x0000000000000000000000000000000000010001"
)

// SystemContractSet contains the system contracts activated at a specific fork.
type SystemContractSet map[common.Address]bool

// SystemContractsGenesis contains the default set of system contracts available from genesis.
var SystemContractsGenesis = SystemContractSet{
	common.HexToAddress(ValidatorContract):       true,
	common.HexToAddress(SlashContract):           true,
	common.HexToAddress(SystemRewardContract):    true,
	common.HexToAddress(LightClientContract):     true,
	common.HexToAddress(RelayerHubContract):      true,
	common.HexToAddress(CandidateHubContract):    true,
	common.HexToAddress(GovHubContract):          true,
	common.HexToAddress(PledgeCandidateContract): true,
	common.HexToAddress(BurnContract):            true,
	common.HexToAddress(FoundationContract):      true,
}

// SystemContractsDemeter contains the system contracts available from the Demeter fork.
var SystemContractsDemeter = SystemContractSet{
	common.HexToAddress(ValidatorContract):       true,
	common.HexToAddress(SlashContract):           true,
	common.HexToAddress(SystemRewardContract):    true,
	common.HexToAddress(LightClientContract):     true,
	common.HexToAddress(RelayerHubContract):      true,
	common.HexToAddress(CandidateHubContract):    true,
	common.HexToAddress(GovHubContract):          true,
	common.HexToAddress(PledgeCandidateContract): true,
	common.HexToAddress(BurnContract):            true,
	common.HexToAddress(FoundationContract):      true,
	common.HexToAddress(StakeHubContract):        true,
	common.HexToAddress(CoreAgentContract):       true,
	common.HexToAddress(HashAgentContract):       true,
	common.HexToAddress(BTCAgentContract):        true,
	common.HexToAddress(BTCStakeContract):        true,
	common.HexToAddress(BTCLSTStakeContract):     true,
	common.HexToAddress(BTCLSTTokenContract):     true,
}

// SystemContractsTheseus contains the system contracts available from the Theseus fork.
var SystemContractsTheseus = SystemContractSet{
	common.HexToAddress(ValidatorContract):       true,
	common.HexToAddress(SlashContract):           true,
	common.HexToAddress(SystemRewardContract):    true,
	common.HexToAddress(LightClientContract):     true,
	common.HexToAddress(RelayerHubContract):      true,
	common.HexToAddress(CandidateHubContract):    true,
	common.HexToAddress(GovHubContract):          true,
	common.HexToAddress(PledgeCandidateContract): true,
	common.HexToAddress(BurnContract):            true,
	common.HexToAddress(FoundationContract):      true,
	common.HexToAddress(StakeHubContract):        true,
	common.HexToAddress(CoreAgentContract):       true,
	common.HexToAddress(HashAgentContract):       true,
	common.HexToAddress(BTCAgentContract):        true,
	common.HexToAddress(BTCStakeContract):        true,
	common.HexToAddress(BTCLSTStakeContract):     true,
	common.HexToAddress(BTCLSTTokenContract):     true,
	common.HexToAddress(FeeMarketContract):       true,
}

var (
	SystemContractAddressesGenesis []common.Address
	SystemContractAddressesDemeter []common.Address
	SystemContractAddressesTheseus []common.Address
)

func init() {
	for k := range SystemContractsGenesis {
		SystemContractAddressesGenesis = append(SystemContractAddressesGenesis, k)
	}
	for k := range SystemContractsDemeter {
		SystemContractAddressesDemeter = append(SystemContractAddressesDemeter, k)
	}
	for k := range SystemContractsTheseus {
		SystemContractAddressesTheseus = append(SystemContractAddressesTheseus, k)
	}
}

func activeSystemContracts(rules params.Rules) SystemContractSet {
	if !rules.IsSatoshi {
		return SystemContractSet{}
	}
	switch {
	case rules.IsTheseus:
		return SystemContractsTheseus
	case rules.IsDemeter:
		return SystemContractsDemeter
	default:
		return SystemContractsGenesis
	}
}

// ActiveSystemContracts returns a copy of system contracts enabled with the current configuration.
func ActiveSystemContracts(rules params.Rules) SystemContractSet {
	return maps.Clone(activeSystemContracts(rules))
}

// ActiveSystemContractAddresses returns the system contract addresses enabled with the current configuration.
func ActiveSystemContractAddresses(rules params.Rules) []common.Address {
	if !rules.IsSatoshi {
		return []common.Address{}
	}
	switch {
	case rules.IsTheseus:
		return SystemContractAddressesTheseus
	case rules.IsDemeter:
		return SystemContractAddressesDemeter
	default:
		return SystemContractAddressesGenesis
	}
}

// IsSystemContract checks if the given address is a system contract for the given configuration.
func IsSystemContract(addr common.Address, rules params.Rules) bool {
	activeContracts := ActiveSystemContracts(rules)
	return activeContracts[addr]
}
