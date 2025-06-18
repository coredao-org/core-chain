package poseidon

import _ "embed"

// contract codes for Mainnet upgrade
var (
	//go:embed mainnet/ValidatorContract
	MainnetValidatorContract string
	//go:embed mainnet/LightClientContract
	MainnetLightClientContract string
	//go:embed mainnet/CandidateHubContract
	MainnetCandidateHubContract string
	//go:embed mainnet/PledgeCandidateContract
	MainnetPledgeCandidateContract string
)
