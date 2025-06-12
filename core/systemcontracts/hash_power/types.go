package hash_power

import _ "embed"

// contract codes for Buffalo upgrade
var (
	//go:embed buffalo/ValidatorContract
	BuffaloValidatorContract string
	//go:embed buffalo/SlashContract
	BuffaloSlashContract string
	//go:embed buffalo/SystemRewardContract
	BuffaloSystemRewardContract string
	//go:embed buffalo/LightClientContract
	BuffaloLightClientContract string
	//go:embed buffalo/RelayerHubContract
	BuffaloRelayerHubContract string
	//go:embed buffalo/CandidateHubContract
	BuffaloCandidateHubContract string
	//go:embed buffalo/GovHubContract
	BuffaloGovHubContract string
	//go:embed buffalo/PledgeCandidateContract
	BuffaloPledgeCandidateContract string
	//go:embed buffalo/BurnContract
	BuffaloBurnContract string
	//go:embed buffalo/FoundationContract
	BuffaloFoundationContract string
)
