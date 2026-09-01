package systemcontracts

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

func TestCoreRewardFixUpgradeEmbed(t *testing.T) {
	for _, net := range []string{mainNet, pigeonNet} {
		up := coreRewardFixUpgrade[net]
		if up == nil || len(up.Configs) != 1 {
			t.Fatalf("coreRewardFixUpgrade[%s] not populated: %+v", net, up)
		}
		cfg := up.Configs[0]
		if cfg.ContractAddr != common.HexToAddress(ValidatorContract) {
			t.Fatalf("[%s] wrong contract addr: %s", net, cfg.ContractAddr)
		}
		code, err := hex.DecodeString(strings.TrimSpace(cfg.Code))
		if err != nil {
			t.Fatalf("[%s] code does not decode: %v", net, err)
		}
		// Must be a non-trivial contract starting with the EVM dispatcher prefix.
		if len(code) < 20000 {
			t.Errorf("[%s] unexpected code length: %d bytes", net, len(code))
		}
		if hex.EncodeToString(code[:4]) != "60806040" {
			t.Errorf("[%s] unexpected code prefix: %x", net, code[:4])
		}
	}
}

func TestIsOnCoreRewardFixTransition(t *testing.T) {
	const forkT = uint64(1788336000) // 2026-09-02 08:00:00 UTC
	c := &params.ChainConfig{
		ChainID:             big.NewInt(1116),
		LondonBlock:         big.NewInt(0),
		CoreRewardFixTime:   &[]uint64{forkT}[0],
	}
	num := big.NewInt(1000)
	// parent before fork, current at/after fork -> transition true
	if !c.IsOnCoreRewardFix(num, forkT-3, forkT) {
		t.Error("expected transition true at the boundary block")
	}
	// both before fork -> false
	if c.IsOnCoreRewardFix(num, forkT-6, forkT-3) {
		t.Error("expected false before fork")
	}
	// both after fork -> false (already applied)
	if c.IsOnCoreRewardFix(num, forkT, forkT+3) {
		t.Error("expected false after fork")
	}
}
