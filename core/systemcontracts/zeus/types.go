package zeus

import _ "embed"

// contract codes for Buffalo upgrade
var (
	//go:embed buffalo/ValidatorContract
	BuffaloValidatorContract string
	//go:embed buffalo/LightClientContract
	BuffaloLightClientContract string
	//go:embed buffalo/CandidateHubContract
	BuffaloCandidateHubContract string
	//go:embed buffalo/GovHubContract
	BuffaloGovHubContract string
	//go:embed buffalo/PledgeCandidateContract
	BuffaloPledgeCandidateContract string
	//go:embed buffalo/FoundationContract
	BuffaloFoundationContract string
)

// contract codes for Mainnet upgrade
var (
	//go:embed mainnet/ValidatorContract
	MainnetValidatorContract string
	//go:embed mainnet/LightClientContract
	MainnetLightClientContract string
	//go:embed mainnet/CandidateHubContract
	MainnetCandidateHubContract string
	//go:embed mainnet/GovHubContract
	MainnetGovHubContract string
	//go:embed mainnet/PledgeCandidateContract
	MainnetPledgeCandidateContract string
	//go:embed mainnet/FoundationContract
	MainnetFoundationContract string
)
