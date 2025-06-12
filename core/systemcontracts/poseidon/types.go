package poseidon

import _ "embed"

// contract codes for Buffalo upgrade
var (
	//go:embed buffalo/ValidatorContract
	BuffaloValidatorContract string
	//go:embed buffalo/LightClientContract
	BuffaloLightClientContract string
	//go:embed buffalo/CandidateHubContract
	BuffaloCandidateHubContract string
	//go:embed buffalo/PledgeCandidateContract
	BuffaloPledgeCandidateContract string
)

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
