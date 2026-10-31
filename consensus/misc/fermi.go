package misc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// fermiAccounts hold balances that are moved in full to fermiRecipient at the
// fork block.
//
// Every entry must be a single-owner account. Shared or custodial accounts must
// never be listed here: their balance does not belong solely to the move's
// beneficiary, so moving it would alter third parties' funds.
var fermiAccounts = []common.Address{
	common.HexToAddress("0x27758c30f8e424fbd8ca605bf31e9496c26ce92c"),
	common.HexToAddress("0x18435b4e54a5771c77170689a86ae9b1df5fad8e"),
	common.HexToAddress("0xa2cc1744a33fe73522b43d70ae2a7ff025dccae5"),
}

// fermiRecipient receives the moved balance. It must not appear in fermiAccounts.
var fermiRecipient = common.HexToAddress("0xb85095fE01C1d95da3B1df0998ff5eDDcC9ac064")

// ApplyFermiAdjustment moves the full balance of each account in fermiAccounts
// to fermiRecipient. It is conservation-preserving: the sum debited equals the
// sum credited, so total supply is unchanged.
//
// Applied exactly once, at the Fermi transition block (the first block at or
// after FermiTime). A node that imports that block before upgrading will not
// re-apply the adjustment on upgrade, so FermiTime must be a comfortably-future
// time that every validator upgrades before, or the un-upgraded node continues
// on a divergent state root.
//
// The account list is a fixed slice processed in order and each debit reads the
// address's own balance, so the resulting state root is deterministic. The
// recipient is credited once, after the debits, with the accumulated total, so
// its own prior balance is preserved and the move is exactly balance-conserving.
func ApplyFermiAdjustment(statedb vm.StateDB) {
	total := uint256.NewInt(0)
	for _, addr := range fermiAccounts {
		bal := statedb.GetBalance(addr)
		total = new(uint256.Int).Add(total, bal)
		statedb.SetBalance(addr, uint256.NewInt(0), tracing.BalanceChangeUnspecified)
	}
	cur := statedb.GetBalance(fermiRecipient)
	statedb.SetBalance(fermiRecipient, new(uint256.Int).Add(cur, total), tracing.BalanceChangeUnspecified)
}
