package luban

import _ "embed"

// contract codes for Default upgrade
var (
	//go:embed default/ValidatorContract
	DefaultValidatorContract string
	//go:embed default/SlashContract
	DefaultSlashContract string
	//go:embed default/CandidateHubContract
	DefaultCandidateHubContract string
)
