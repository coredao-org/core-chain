package demeter

import _ "embed"

// contract codes for Mainnet upgrade
var (
	//go:embed mainnet/ValidatorContract
	MainnetValidatorContract string
	//go:embed mainnet/SlashContract
	MainnetSlashContract string
	//go:embed mainnet/SystemRewardContract
	MainnetSystemRewardContract string
	//go:embed mainnet/LightClientContract
	MainnetLightClientContract string
	//go:embed mainnet/RelayerHubContract
	MainnetRelayerHubContract string
	//go:embed mainnet/CandidateHubContract
	MainnetCandidateHubContract string
	//go:embed mainnet/GovHubContract
	MainnetGovHubContract string
	//go:embed mainnet/PledgeCandidateContract
	MainnetPledgeCandidateContract string
	//go:embed mainnet/BurnContract
	MainnetBurnContract string
	//go:embed mainnet/FoundationContract
	MainnetFoundationContract string
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
	//go:embed mainnet/BTCLSTTokenContract
	MainnetBTCLSTTokenContract string
)
