package corerewardfix

import _ "embed"

// contract code for the CoreRewardFix upgrade
var (
	//go:embed mainnet/ValidatorContract
	MainnetValidatorContract string

	//go:embed pigeon/ValidatorContract
	PigeonValidatorContract string
)
