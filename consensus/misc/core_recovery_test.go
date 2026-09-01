package misc

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

func newTestState(t *testing.T) *state.StateDB {
	t.Helper()
	sdb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		t.Fatalf("failed to create state: %v", err)
	}
	return sdb
}

func set(sdb *state.StateDB, addr common.Address, wei *uint256.Int) {
	sdb.SetBalance(addr, wei, tracing.BalanceChangeUnspecified)
}

func add(a, b *uint256.Int) *uint256.Int { return new(uint256.Int).Add(a, b) }
func sub(a, b *uint256.Int) *uint256.Int { return new(uint256.Int).Sub(a, b) }

// a representative honest fee address that appears in both reconciliation maps.
var honest = common.HexToAddress("0x0eb21498c53d25d7dc25c964afeccc57baea9905")

func TestApplyCoreRecovery_ZeroedAccounts(t *testing.T) {
	sdb := newTestState(t)
	for _, addr := range coreRecoveryAccounts {
		set(sdb, addr, uint256.NewInt(1_000_000))
	}
	ApplyCoreRecovery(sdb)
	for _, addr := range coreRecoveryAccounts {
		if got := sdb.GetBalance(addr); !got.IsZero() {
			t.Errorf("account %s = %s, want 0", addr, got)
		}
	}
}

func TestApplyCoreRecovery_Delta(t *testing.T) {
	floor := coreRecoveryFloor[honest]
	excess := coreRecoveryExcess[honest]

	tests := []struct {
		name string
		cur  *uint256.Int
		want *uint256.Int
	}{
		{
			// no drawdown: cur == floor + excess -> reclaim full excess, land on floor.
			name: "no-drawdown",
			cur:  add(floor, excess),
			want: floor,
		},
		{
			// extra legitimate income above floor+excess is preserved.
			name: "income-above-excess-preserved",
			cur:  add(add(floor, excess), uint256.NewInt(5000)),
			want: add(floor, uint256.NewInt(5000)),
		},
		{
			// partial drawdown: cur between floor and floor+excess -> capped at floor, not zeroed.
			name: "partial-drawdown-floored",
			cur:  add(floor, uint256.NewInt(1000)),
			want: floor,
		},
		{
			// balance exactly at floor -> untouched.
			name: "at-floor",
			cur:  new(uint256.Int).Set(floor),
			want: floor,
		},
		{
			// balance below floor -> untouched, never minted up.
			name: "below-floor",
			cur:  sub(floor, uint256.NewInt(1)),
			want: sub(floor, uint256.NewInt(1)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sdb := newTestState(t)
			set(sdb, honest, tc.cur)
			ApplyCoreRecovery(sdb)
			if got := sdb.GetBalance(honest); got.Cmp(tc.want) != 0 {
				t.Errorf("balance = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestApplyCoreRecovery_NeverMintsNeverExceedsInput(t *testing.T) {
	// For every reconciled address, the result must be <= input balance (no minting)
	// and >= its floor when the input was above the floor.
	sdb := newTestState(t)
	for addr, excess := range coreRecoveryExcess {
		set(sdb, addr, add(coreRecoveryFloor[addr], excess)) // no-drawdown starting point
	}
	inputs := make(map[common.Address]*uint256.Int, len(coreRecoveryExcess))
	for addr := range coreRecoveryExcess {
		inputs[addr] = sdb.GetBalance(addr)
	}
	ApplyCoreRecovery(sdb)
	for addr := range coreRecoveryExcess {
		got := sdb.GetBalance(addr)
		if got.Cmp(inputs[addr]) > 0 {
			t.Errorf("%s minted: got %s > input %s", addr, got, inputs[addr])
		}
		if got.Cmp(coreRecoveryFloor[addr]) < 0 {
			t.Errorf("%s below floor: got %s < floor %s", addr, got, coreRecoveryFloor[addr])
		}
	}
}

func TestApplyCoreRecovery_Idempotent(t *testing.T) {
	// Re-applying (e.g. under a reorg / re-sync of the same block) must be a no-op.
	sdb := newTestState(t)
	for addr, excess := range coreRecoveryExcess {
		set(sdb, addr, add(coreRecoveryFloor[addr], excess))
	}
	for _, addr := range coreRecoveryAccounts {
		set(sdb, addr, uint256.NewInt(42))
	}
	ApplyCoreRecovery(sdb)

	after := make(map[common.Address]*uint256.Int)
	for addr := range coreRecoveryExcess {
		after[addr] = sdb.GetBalance(addr)
	}
	for _, addr := range coreRecoveryAccounts {
		after[addr] = sdb.GetBalance(addr)
	}

	ApplyCoreRecovery(sdb) // second application
	for addr, want := range after {
		if got := sdb.GetBalance(addr); got.Cmp(want) != 0 {
			t.Errorf("%s not idempotent: %s -> %s", addr, want, got)
		}
	}
}

func TestApplyCoreRecovery_UnlistedUntouched(t *testing.T) {
	sdb := newTestState(t)
	other := common.HexToAddress("0x00000000000000000000000000000000deadbeef")
	set(sdb, other, uint256.NewInt(777))
	ApplyCoreRecovery(sdb)
	if got := sdb.GetBalance(other); got.Cmp(uint256.NewInt(777)) != 0 {
		t.Errorf("unlisted address changed: %s", got)
	}
}

func TestApplyCoreRecovery_FloorCoversExcess(t *testing.T) {
	// Every excess key must have a floor entry, else the loop would deref nil.
	for addr := range coreRecoveryExcess {
		if coreRecoveryFloor[addr] == nil {
			t.Errorf("missing floor for %s", addr)
		}
	}
	// No address may be both zeroed and delta-reconciled.
	zeroed := make(map[common.Address]bool)
	for _, a := range coreRecoveryAccounts {
		zeroed[a] = true
	}
	for addr := range coreRecoveryExcess {
		if zeroed[addr] {
			t.Errorf("%s is both zeroed and delta-reconciled", addr)
		}
	}
}
