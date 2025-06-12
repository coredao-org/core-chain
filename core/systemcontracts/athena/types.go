package athena

import _ "embed"

// contract codes for Pigeon upgrade
var (
	//go:embed pigeon/ValidatorContract
	PigeonValidatorContract string
	//go:embed pigeon/SystemRewardContract
	PigeonSystemRewardContract string
	//go:embed pigeon/LightClientContract
	PigeonLightClientContract string
	//go:embed pigeon/CandidateHubContract
	PigeonCandidateHubContract string
	//go:embed pigeon/GovHubContract
	PigeonGovHubContract string
	//go:embed pigeon/PledgeCandidateContract
	PigeonPledgeCandidateContract string
	//go:embed pigeon/BurnContract
	PigeonBurnContract string
	//go:embed pigeon/StakeHubContract
	PigeonStakeHubContract string
	//go:embed pigeon/CoreAgentContract
	PigeonCoreAgentContract string
	//go:embed pigeon/HashAgentContract
	PigeonHashAgentContract string
	//go:embed pigeon/BTCAgentContract
	PigeonBTCAgentContract string
	//go:embed pigeon/BTCStakeContract
	PigeonBTCStakeContract string
	//go:embed pigeon/BTCLSTStakeContract
	PigeonBTCLSTStakeContract string
)

// contract codes for Mainnet upgrade
var (
	//go:embed mainnet/ValidatorContract
	MainnetValidatorContract string
	//go:embed mainnet/SystemRewardContract
	MainnetSystemRewardContract string
	//go:embed mainnet/LightClientContract
	MainnetLightClientContract string
	//go:embed mainnet/CandidateHubContract
	MainnetCandidateHubContract string
	//go:embed mainnet/GovHubContract
	MainnetGovHubContract string
	//go:embed mainnet/PledgeCandidateContract
	MainnetPledgeCandidateContract string
	//go:embed mainnet/BurnContract
	MainnetBurnContract string
	//go:embed mainnet/StakeHubContract
	MainnetStakeHubContract string
	//go:embed mainnet/CoreAgentContract
	MainnetCoreAgentContract string
	//go:embed mainnet/HashAgentContract
	MainnetHashAgentContract string
	//go:embed mainnet/BTCAgentContract
	MainnetBTCAgentContract string
	//go:embed mainnet/BTCStakeContract
	MainnetBTCStakeContract string
	//go:embed mainnet/BTCLSTStakeContract
	MainnetBTCLSTStakeContract string
)
