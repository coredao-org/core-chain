package hera

import _ "embed"

// contract codes for Buffalo upgrade
var (
	//go:embed buffalo/LightClientContract
	BuffaloLightClientContract string
)

// contract codes for Mainnet upgrade
var (
	//go:embed mainnet/LightClientContract
	MainnetLightClientContract string
)
