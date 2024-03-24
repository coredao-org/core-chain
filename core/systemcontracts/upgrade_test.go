package systemcontracts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
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
	err := checkUpgradeConfig("https://raw.githubusercontent.com/coredao-org/core-genesis-contract/branch_testnet/genesis.json", poseidonUpgrade[buffaloNet])
	if err != nil {
		t.Error(err)
	}
}
