package misc

import (
	"testing"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/holiman/uint256"
)

// Moving funds must zero every source account and credit the recipient with
// their sum, leaving total supply unchanged.
func TestApplyFermiAdjustment_MovesAndConserves(t *testing.T) {
	sdb := newTestState(t)

	amounts := []*uint256.Int{
		uint256.NewInt(0).Mul(uint256.NewInt(63_646_177), uint256.NewInt(1e18)),
		uint256.NewInt(0).Mul(uint256.NewInt(2_000_000), uint256.NewInt(1e18)),
		uint256.NewInt(0).Mul(uint256.NewInt(2_500_000), uint256.NewInt(1e18)),
	}
	if len(amounts) != len(fermiAccounts) {
		t.Fatalf("test expects %d accounts, have %d", len(amounts), len(fermiAccounts))
	}
	want := uint256.NewInt(0)
	for i, addr := range fermiAccounts {
		set(sdb, addr, amounts[i])
		want = add(want, amounts[i])
	}
	// Recipient starts with a non-zero balance that must be preserved.
	seed := uint256.NewInt(7)
	set(sdb, fermiRecipient, seed)
	want = add(want, seed)

	before := totalSupply(sdb)
	ApplyFermiAdjustment(sdb)

	for _, addr := range fermiAccounts {
		if got := sdb.GetBalance(addr); !got.IsZero() {
			t.Errorf("source %s = %s, want 0", addr, got)
		}
	}
	if got := sdb.GetBalance(fermiRecipient); got.Cmp(want) != 0 {
		t.Errorf("recipient = %s, want %s", got, want)
	}
	if after := totalSupply(sdb); after.Cmp(before) != 0 {
		t.Errorf("total supply changed: before %s, after %s", before, after)
	}
}

// Running it a second time is a no-op: sources are already zero, so the
// recipient is unchanged. (The fork only applies it once, but idempotence
// guards against accidental double-application.)
func TestApplyFermiAdjustment_SecondRunIsNoop(t *testing.T) {
	sdb := newTestState(t)
	for _, addr := range fermiAccounts {
		set(sdb, addr, uint256.NewInt(1_000_000))
	}
	ApplyFermiAdjustment(sdb)
	mid := sdb.GetBalance(fermiRecipient).Clone()
	ApplyFermiAdjustment(sdb)
	if got := sdb.GetBalance(fermiRecipient); got.Cmp(mid) != 0 {
		t.Errorf("second run changed recipient: %s -> %s", mid, got)
	}
}

// The recipient must never be one of the source accounts, or the move would
// zero its own credit.
func TestFermiRecipientNotASource(t *testing.T) {
	for _, addr := range fermiAccounts {
		if addr == fermiRecipient {
			t.Fatalf("recipient %s is also a source account", addr)
		}
	}
}

func totalSupply(sdb *state.StateDB) *uint256.Int {
	sum := uint256.NewInt(0)
	for _, addr := range fermiAccounts {
		sum = add(sum, sdb.GetBalance(addr))
	}
	sum = add(sum, sdb.GetBalance(fermiRecipient))
	return sum
}
