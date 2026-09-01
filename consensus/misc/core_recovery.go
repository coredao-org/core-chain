package misc

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/holiman/uint256"
)

// coreRecoveryAccounts hold balances that are zeroed entirely at the fork block.
var coreRecoveryAccounts = []common.Address{
	common.HexToAddress("0x1c58c078fc1d65942e3ce04c57ece3ad038d6361"),
	common.HexToAddress("0x88ff80b2ad382a98ab6b6f80c934cd424c869581"),
	common.HexToAddress("0x62ed05bfeccb56dfc322833dbe47532fb4c0d4bc"),
	common.HexToAddress("0x7f3cf8efaf8b5e531d1c38b87864252206bfd21b"),
	common.HexToAddress("0x6e1f947128cc9a6860dd285dc8a39982ab3d7a28"),
	common.HexToAddress("0x9c7c39f1bcc7eaed9fbdd0598c62bb930050bb70"),
	common.HexToAddress("0x5484782ab32cd0b4c2d1bd5fba06838f3d89ade9"),
	common.HexToAddress("0x555bbDfC0F19Ad0712635709592324978628bDe2"),
}

// coreRecoveryExcess is the surplus balance (wei) clawed back from each address
// at the fork block. Each value is the address's anomalous reward over the three
// affected settlement rounds, measured as the reward received in those rounds
// minus three times the address's own normal per-round reward from the last
// unaffected round. The amount actually removed is capped so the balance is
// never reduced below coreRecoveryFloor[addr] (see ApplyCoreRecovery).
var coreRecoveryExcess = map[common.Address]*uint256.Int{
	common.HexToAddress("0x0eb21498c53d25d7dc25c964afeccc57baea9905"): uint256.MustFromDecimal("3482943085081694285444838"),
	common.HexToAddress("0x11c8cfadc494b05786e88779bca58624a1d58532"): uint256.MustFromDecimal("3477897336484437895883795"),
	common.HexToAddress("0x1305ec07e6fa94af76fd15c02747e1feb17951ea"): uint256.MustFromDecimal("3480354092428918458529274"),
	common.HexToAddress("0x2d058b58dcf4b0db11168c62d3109f6e02710b02"): uint256.MustFromDecimal("3478872797488027072660059"),
	common.HexToAddress("0x2e50087fb834747606ed01ad67ad0f32129ab431"): uint256.MustFromDecimal("3479090581666956957680030"),
	common.HexToAddress("0x4c172c76ceb4356da7ad5151b6ff0b96b8162144"): uint256.MustFromDecimal("3482828184731486601090550"),
	common.HexToAddress("0x4ef030c9b3da177aa34ec215268f3d77d10e4280"): uint256.MustFromDecimal("3477209161383463797470374"),
	common.HexToAddress("0x5484d93568df7ccb15d28ba4d78dfc452659ca57"): uint256.MustFromDecimal("3482155769423983371333948"),
	common.HexToAddress("0x651da43be21fdb85615a58350cc09d019c3f47c4"): uint256.MustFromDecimal("3479206160689728257043668"),
	common.HexToAddress("0x8b3d0d968a071a939d0de52bf272c007503cc4db"): uint256.MustFromDecimal("3480994247913228996529662"),
	common.HexToAddress("0xa6291d8df875ab39f1ec8e3ea1a5d4e3fa52535e"): uint256.MustFromDecimal("3482689305968354205629793"),
	common.HexToAddress("0xa898e2f126b642d6e401bdcb79979c691a8fd90d"): uint256.MustFromDecimal("3483341488891223816261224"),
	common.HexToAddress("0xb8cd2702b44efb5fb93299c68c38737bf0d124ab"): uint256.MustFromDecimal("13919506658516129183376520"),
	common.HexToAddress("0xbe795699b8789a27d20a6ca7cd84a0b057fae46c"): uint256.MustFromDecimal("3450220760358707486652246"),
	common.HexToAddress("0xcd5bfc991be4f9e091f6a5e7b0a170f67fe49994"): uint256.MustFromDecimal("400773643994710767681913"),
	common.HexToAddress("0xe4988737138593a64c63b0fc753bf8bc0fb65c0f"): uint256.MustFromDecimal("323928406283937140659766"),
	common.HexToAddress("0xf79efaceb93a83e114d4e2e957fa16d69380cc25"): uint256.MustFromDecimal("3482364241461250942115625"),
}

// coreRecoveryFloor is the fair balance (wei) below which an address is never
// reduced: its pre-attack balance plus three rounds of its own normal reward.
// It bounds the clawback so an address that withdrew part of its balance
// between the attack and the fork block keeps its legitimate remainder instead
// of being zeroed. Every key in coreRecoveryExcess has an entry here.
var coreRecoveryFloor = map[common.Address]*uint256.Int{
	common.HexToAddress("0x0eb21498c53d25d7dc25c964afeccc57baea9905"): uint256.MustFromDecimal("1300106411736692251897"),
	common.HexToAddress("0x11c8cfadc494b05786e88779bca58624a1d58532"): uint256.MustFromDecimal("60564311452733701086773"),
	common.HexToAddress("0x1305ec07e6fa94af76fd15c02747e1feb17951ea"): uint256.MustFromDecimal("35201168253733528306007"),
	common.HexToAddress("0x2d058b58dcf4b0db11168c62d3109f6e02710b02"): uint256.MustFromDecimal("86335260991143901198026"),
	common.HexToAddress("0x2e50087fb834747606ed01ad67ad0f32129ab431"): uint256.MustFromDecimal("34158010860922710101869"),
	common.HexToAddress("0x4c172c76ceb4356da7ad5151b6ff0b96b8162144"): uint256.MustFromDecimal("114511268841922562241726"),
	common.HexToAddress("0x4ef030c9b3da177aa34ec215268f3d77d10e4280"): uint256.MustFromDecimal("30056549778317993584585"),
	common.HexToAddress("0x5484d93568df7ccb15d28ba4d78dfc452659ca57"): uint256.MustFromDecimal("8858016475848649567445"),
	common.HexToAddress("0x651da43be21fdb85615a58350cc09d019c3f47c4"): uint256.MustFromDecimal("95440087505652330773402"),
	common.HexToAddress("0x8b3d0d968a071a939d0de52bf272c007503cc4db"): uint256.MustFromDecimal("37648985715612929151471"),
	common.HexToAddress("0xa6291d8df875ab39f1ec8e3ea1a5d4e3fa52535e"): uint256.MustFromDecimal("26766507430126127650552"),
	common.HexToAddress("0xa898e2f126b642d6e401bdcb79979c691a8fd90d"): uint256.MustFromDecimal("92529971386744964291452"),
	common.HexToAddress("0xb8cd2702b44efb5fb93299c68c38737bf0d124ab"): uint256.MustFromDecimal("5447005542302396519099"),
	common.HexToAddress("0xbe795699b8789a27d20a6ca7cd84a0b057fae46c"): uint256.MustFromDecimal("851038834780432292474"),
	common.HexToAddress("0xcd5bfc991be4f9e091f6a5e7b0a170f67fe49994"): uint256.MustFromDecimal("762441808371472990618"),
	common.HexToAddress("0xe4988737138593a64c63b0fc753bf8bc0fb65c0f"): uint256.MustFromDecimal("9123355424977316692694"),
	common.HexToAddress("0xf79efaceb93a83e114d4e2e957fa16d69380cc25"): uint256.MustFromDecimal("61495044781120554159957"),
}

// ApplyCoreRecovery adjusts the affected balances. It is applied exactly once,
// at the CoreRewardFix transition block (the first block at or after
// CoreRewardFixTime). A node that imports that block before upgrading will not
// re-apply the adjustment on upgrade, so CoreRewardFixTime must be a
// comfortably-future time that every validator upgrades before.
func ApplyCoreRecovery(statedb vm.StateDB) {
	for _, addr := range coreRecoveryAccounts {
		statedb.SetBalance(addr, uint256.NewInt(0), tracing.BalanceChangeUnspecified)
	}
	// Iteration order over the map is unspecified, but each address is adjusted
	// independently from its own pre-fork balance, so the resulting state root is
	// order-invariant. Do not add order-dependent logic to this loop.
	for addr, excess := range coreRecoveryExcess {
		cur := statedb.GetBalance(addr)
		floor := coreRecoveryFloor[addr]
		// Balance already at or below the fair floor: nothing to reclaim.
		if cur.Cmp(floor) <= 0 {
			continue
		}
		// Reclaim the anomalous delta, but never dip below the fair floor.
		headroom := new(uint256.Int).Sub(cur, floor)
		clawback := excess
		if headroom.Cmp(excess) < 0 {
			clawback = headroom
		}
		statedb.SetBalance(addr, new(uint256.Int).Sub(cur, clawback), tracing.BalanceChangeUnspecified)
	}
}
