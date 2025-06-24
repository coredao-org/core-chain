package systemcontracts

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

type Config struct {
	ChainId             int
	HomesteadBlock      int
	Eip150Block         int
	Eip150Hash          string
	Eip155Block         int
	Eip158Block         int
	ByzantiumBlock      int
	ConstantinopleBlock int
	PetersburgBlock     int
	IstanbulBlock       int
	MuirGlacierBlock    int
	HashPowerBlock      int
	Satoshi             struct {
		Period int
		Epoch  int
		Round  int
	}
}

type Alloc struct {
	Balance string
	Code    string
}

type Genesis struct {
	Config     Config
	Nonce      string
	Timestamp  string
	ExtraData  string
	GasLimit   string
	Difficulty string
	MixHash    string
	Coinbase   string
	Alloc      map[string]Alloc
	Number     string
	GasUsed    string
	ParentHash string
}

func checkUpgradeConfig(url string, upgrade *Upgrade) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Error fetching file: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Error reading response: %v", err)
	}

	var genesis Genesis
	err = json.Unmarshal(body, &genesis)
	if err != nil {
		return fmt.Errorf("Error parsing JSON: %v", err)
	}

	for _, config := range upgrade.Configs {
		addr := config.ContractAddr
		genesisCode := genesis.Alloc[addr.String()].Code
		upgradeCode := config.Code
		if genesisCode != "0x"+upgradeCode {
			return fmt.Errorf("Upgrade code mismatch for contract %s: expected %s, got %s", addr.String(), upgradeCode, genesisCode)
		}
	}

	return nil
}

func TestUpgrade(t *testing.T) {
	t.Parallel()

	// Map of upgrade name + network to commit hash for genesis file verification
	testCases := []struct {
		name       string
		upgrade    *Upgrade
		commitHash string
		network    string
	}{
		// Zeus upgrades
		{
			name:       "zeus_mainnet",
			upgrade:    zeusUpgrade[mainNet],
			commitHash: "d545ede09f6af0b2c2451f739234e582f0aeeb2b",
			network:    mainNet,
		},

		// Hera upgrades
		{
			name:       "hera_mainnet",
			upgrade:    heraUpgrade[mainNet],
			commitHash: "6b8aad810e5e023352bedadc022eed9a280b6367",
			network:    mainNet,
		},

		// Poseidon upgrades
		{
			name:       "poseidon_mainnet",
			upgrade:    poseidonUpgrade[mainNet],
			commitHash: "5e846bfd00de9d59ab32005e0bb7916615d8c764",
			network:    mainNet,
		},

		// Demeter upgrades
		{
			name:       "demeter_mainnet",
			upgrade:    demeterUpgrade[mainNet],
			commitHash: "925e416b42cb97ffba3b1bad4394d7e609452e10",
			network:    mainNet,
		},

		// Athena upgrades
		{
			name:       "athena_pigeon",
			upgrade:    athenaUpgrade[pigeonNet],
			commitHash: "76bb9aec72556db2646d449fca9b8832d5829ec1",
			network:    pigeonNet,
		},
		{
			name:       "athena_mainnet",
			upgrade:    athenaUpgrade[mainNet],
			commitHash: "48ab9c0505af2478b0f958d8c4f42a09ba2d072b",
			network:    mainNet,
		},

		// Theseus upgrades
		{
			name:       "theseus_pigeon",
			upgrade:    theseusUpgrade[pigeonNet],
			commitHash: "15121cba9d0e6dcb99c144118539484abfb1240b",
			network:    pigeonNet,
		},
		{
			name:       "theseus_mainnet",
			upgrade:    theseusUpgrade[mainNet],
			commitHash: "0689a1e5d69d25f0d74c2363dee640d790130817",
			network:    mainNet,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.upgrade == nil {
				t.Errorf("Upgrade config for %s is nil", tc.name)
				return
			}

			// Verify commit URLs in upgrade config match expected commit hash
			for _, config := range tc.upgrade.Configs {
				expectedCommitUrl := fmt.Sprintf("https://github.com/coredao-org/core-genesis-contract/commit/%s", tc.commitHash)
				if config.CommitUrl != expectedCommitUrl {
					t.Errorf("Commit URL mismatch for %s: expected %s, got %s",
						tc.name, expectedCommitUrl, config.CommitUrl)
				}
			}

			// Test against genesis file from commit
			genesisUrl := fmt.Sprintf("https://raw.githubusercontent.com/coredao-org/core-genesis-contract/%s/genesis.json", tc.commitHash)
			err := checkUpgradeConfig(genesisUrl, tc.upgrade)
			if err != nil {
				t.Errorf("Upgrade config verification failed for %s: %v", tc.name, err)
			}
		})
	}
}

func TestUpgradeBuildInSystemContractNilInterface(t *testing.T) {
	var (
		config               = params.CoreChainConfig
		blockNumber          = big.NewInt(37959559)
		lastBlockTime uint64 = 1713419337
		blockTime     uint64 = 1713419340
		statedb       vm.StateDB
	)

	GenesisHash = params.CoreGenesisHash

	upgradeBuildInSystemContract(config, blockNumber, lastBlockTime, blockTime, statedb)
}

func TestUpgradeBuildInSystemContractNilValue(t *testing.T) {
	var (
		config                   = params.CoreChainConfig
		blockNumber              = big.NewInt(37959559)
		lastBlockTime uint64     = 1713419337
		blockTime     uint64     = 1713419340
		statedb       vm.StateDB = (*state.StateDB)(nil)
	)

	GenesisHash = params.CoreGenesisHash

	upgradeBuildInSystemContract(config, blockNumber, lastBlockTime, blockTime, statedb)
}
