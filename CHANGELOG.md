# Changelog
<<<<<<< HEAD
=======
## v1.5.12
### BUGFIX
[\#3057](https://github.com/bnb-chain/bsc/pull/3057) eth/protocols/bsc: adjust vote reception limit

## v1.5.11
### FEATURE
[\#3008](https://github.com/bnb-chain/bsc/pull/3008) params: add MaxwellTime
[\#3025](https://github.com/bnb-chain/bsc/pull/3025) config.toml: default value of [Eth.Miner] and [Eth.Miner.Mev]
[\#3026](https://github.com/bnb-chain/bsc/pull/3026) mev: include MaxBidsPerBuilder in MevParams
[\#3027](https://github.com/bnb-chain/bsc/pull/3027) mev: update two default mev paramater for 1.5s block interval

### BUGFIX
[\#3007](https://github.com/bnb-chain/bsc/pull/3007) metrics: fix panic for cocurrently accessing label

### IMPROVEMENT
[\#3006](https://github.com/bnb-chain/bsc/pull/3006) jsutil: update getKeyParameters
[\#3018](https://github.com/bnb-chain/bsc/pull/3018) nancy: update nancy ignore
[\#3021](https://github.com/bnb-chain/bsc/pull/3021) chore: remove duplicate package imports

## v1.5.10
### FEATURE
[\#3015](https://github.com/bnb-chain/bsc/pull/3015) config: update BSC Mainnet hardfork time: Lorentz

### BUGFIX
NA

### IMPROVEMENT
[\#2985](https://github.com/bnb-chain/bsc/pull/2985) core: clearup pascal&prague testflag and rialto code

## v1.5.9
### FEATURE
[\#2932](https://github.com/bnb-chain/bsc/pull/2932) BEP-520: Short Block Interval Phase One: 1.5 seconds
[\#2991](https://github.com/bnb-chain/bsc/pull/2991) config: update BSC Testnet hardfork time: Lorentz

### BUGFIX
[\#2990](https://github.com/bnb-chain/bsc/pull/2990) core/state: fix concurrent map read and write for stateUpdate.accounts

### IMPROVEMENT
[\#2933](https://github.com/bnb-chain/bsc/pull/2933) metrics: add more peer, block/vote metrics
[\#2938](https://github.com/bnb-chain/bsc/pull/2938) cmd/geth: add example for geth bls account generate-proof
[\#2949](https://github.com/bnb-chain/bsc/pull/2949) metrics: add more block/vote stats;
[\#2948](https://github.com/bnb-chain/bsc/pull/2948) go.mod: update crypto to solve CVE-2025-22869
[\#2960](https://github.com/bnb-chain/bsc/pull/2960) pool: debug log instead of warn
[\#2961](https://github.com/bnb-chain/bsc/pull/2961) metric: add more block monitor metrics;
[\#2992](https://github.com/bnb-chain/bsc/pull/2992) core/systemcontracts: update url for lorentz hardfork
[\#2993](https://github.com/bnb-chain/bsc/pull/2993) cmd/jsutils: add tool GetMevStatus

## v1.5.8
### FEATURE
* [\#2955](https://github.com/bnb-chain/bsc/pull/2955) pbs: enable GreedyMergeTx by default

### BUGFIX
* [\#2967](https://github.com/bnb-chain/bsc/pull/2967) fix: gas compare in bid simulator

### IMPROVEMENT
* [\#2951](https://github.com/bnb-chain/bsc/pull/2951) bump golang.org/x/net from 0.34.0 to 0.36.0
* [\#0000](https://github.com/bnb-chain/bsc/pull/0000) golang: upgrade toolchain to v1.23.0 (commit:3be156eec)
* [\#2957](https://github.com/bnb-chain/bsc/pull/2957) miner: stop GreedyMergeTx before worker picking bids
* [\#2959](https://github.com/bnb-chain/bsc/pull/2959) pbs: fix a inaccurate bid result log
* [\#2971](https://github.com/bnb-chain/bsc/pull/2971) mev: no interrupt if it is too later
* [\#2974](https://github.com/bnb-chain/bsc/pull/2974) miner: add metrics for bid simulation

## v1.5.7
v1.5.7 conduct small upstream code merge to follow the latest pectra hard fork and apply some bug fix. There are two PR for the code merge:
* [\#2897](https://github.com/bnb-chain/bsc/pull/2897) upstream: merge tag 'geth-v1.15.1' into bsc-develop
* [\#2926](https://github.com/bnb-chain/bsc/pull/2926) upstream: pick bug fix from latest geth

Besides code merge, there are also several important bugfix/improvements, and setup mainnet Pascal hard fork time:
### FEATURE
* [\#2928](https://github.com/bnb-chain/bsc/pull/2928) config: update BSC Mainnet hardfork date: Pascal & Praque

### BUGFIX
* [\#2907](https://github.com/bnb-chain/bsc/pull/0000) go.mod: downgrade bls-eth-go-binary to make it same as the prysm-v5.0.0

### IMPROVEMENT
* [\#2896](https://github.com/bnb-chain/bsc/pull/2896) consensus/parlia: estimate gas reserved for systemTxs
* [\#2912](https://github.com/bnb-chain/bsc/pull/2912) consensus/parlia: improve performance of func IsSystemTransaction
* [\#2916](https://github.com/bnb-chain/bsc/pull/2916) miner: avoid to collect requests when getting pending blocks
* [\#2913](https://github.com/bnb-chain/bsc/pull/2913) core/vm: add basic test cases for blsSignatureVerify
* [\#2918](https://github.com/bnb-chain/bsc/pull/2918) core/txpool/legacypool/legacypool.go: add gasTip check when reset

## v1.5.6
v1.5.6 performed another small code sync with Geth upstream, mainly sync the 7702 tx type txpool update, refer: https://github.com/bnb-chain/bsc/pull/2888/

And it also setup the Testnet Pascal hard fork date.

## v1.5.5
v1.5.5 mainly did a upstream code sync, it catches up to Geth of date around 5th-Feb-2025 to sync the latest Praque hard fork changes, see: https://github.com/bnb-chain/bsc/pull/2856

Besides the code sync, there are a few improvement PRs of BSC:
### IMPROVEMENT
* [\#2846](https://github.com/bnb-chain/bsc/pull/2846) tracing: pass *tracing.Hooks around instead of vm.Config
* [\#2850](https://github.com/bnb-chain/bsc/pull/2850) metric: revert default expensive metrics in blockchain
* [\#2851](https://github.com/bnb-chain/bsc/pull/2851) eth/tracers: fix call nil OnSystemTxFixIntrinsicGas
* [\#2862](https://github.com/bnb-chain/bsc/pull/2862) cmd/jsutils/getchainstatus.js: add GetEip7623
* [\#2867](https://github.com/bnb-chain/bsc/pull/2867) p2p: lowered log lvl for failed enr request

## v1.5.4
### BUGFIX
* [\#2874](https://github.com/bnb-chain/bsc/pull/2874) crypto: add IsOnCurve check


## v1.5.3
### BUGFIX
* [\#2827](https://github.com/bnb-chain/bsc/pull/2827) triedb/pathdb: fix nil field for stateSet
* [\#2830](https://github.com/bnb-chain/bsc/pull/2830) fastnode: fix some pbss saving&rewind issues
* [\#2835](https://github.com/bnb-chain/bsc/pull/2835) dep: fix nancy issues
* [\#2836](https://github.com/bnb-chain/bsc/pull/2836) Revert "internal/ethapi: remove td field from block (#30386)"

### FEATURE
NA

### IMPROVEMENT
* [\#2834](https://github.com/bnb-chain/bsc/pull/2834) eth: make transaction acceptance depends on syncing status
* [\#2844](https://github.com/bnb-chain/bsc/pull/2844) internal/ethapi: support GetFinalizedBlock by common ratio validators
* [\#2772](https://github.com/bnb-chain/bsc/pull/2772) Push tracing of Parlia system transactions so that live tracers can properly traces those state changes
* [\#2845](https://github.com/bnb-chain/bsc/pull/2845) feat: wait miner finish the later multi-proposals when restarting the node

## v1.5.2
v1.5.2-alpha is another release for upstream code sync, it catches up with [go-ethereum release [v1.14.12]](https://github.com/ethereum/go-ethereum/releases/tag/v1.14.12) and supported 4 BEPs for BSC Pascal hard fork.
- BEP-439: Implement EIP-2537: Precompile for BLS12-381 curve operations
- BEP-440: Implement EIP-2935: Serve historical block hashes from state
- BEP-441: Implement EIP-7702: Set EOA account code
- BEP-466: Make the block format compatible with EIP-7685

#### Code Sync
- [upstream: merge geth v1.14.12](https://github.com/bnb-chain/bsc/pull/2791)
- [upstream: merge geth date 1210](https://github.com/bnb-chain/bsc/pull/2799)
- [upstream: merge geth date 1213](https://github.com/bnb-chain/bsc/pull/2806)

#### Pascal BEPs
- [BEP-441: Implement EIP-7702: Set EOA account code](https://github.com/bnb-chain/bsc/pull/2807)
- [BEP-466: Make the block header format compatible with EIP-7685](https://github.com/bnb-chain/bsc/pull/2777)
- Note: BEP-439 and BEP-440 have already been implemented in previous v1.5.1-alpha release

#### Others
- [eth/fetcher: remove light mode in block fetcher](https://github.com/bnb-chain/bsc/pull/2804)
- [fix: Opt pruneancient issues](https://github.com/bnb-chain/bsc/pull/2800)
- [build(deps): bump github.com/golang-jwt/jwt/v4 from 4.5.0 to 4.5.1](https://github.com/bnb-chain/bsc/pull/2788)
- [build(deps): bump golang.org/x/crypto from 0.22.0 to 0.31.0](https://github.com/bnb-chain/bsc/pull/2797)
- [misc: mini fix and clearup](https://github.com/bnb-chain/bsc/pull/2811)

## v1.5.1
v1.5.1-alpha is for upstream code sync, it catches up with[go-ethereum release [v1.13.15, v1.14.11]](https://github.com/ethereum/go-ethereum/releases)

#### Major Changes
[eth: make transaction propagation paths in the network deterministic (#29034)](https://github.com/ethereum/go-ethereum/pull/29034)
[core/state: parallelise parts of state commit (#29681)](https://github.com/ethereum/go-ethereum/pull/29681)
Load trie nodes concurrently with trie updates, speeding up block import by 5-7% ([#29519](https://github.com/ethereum/go-ethereum/pull/29519), [#29768](https://github.com/ethereum/go-ethereum/pull/29768), [#29919](https://github.com/ethereum/go-ethereum/pull/29919)).
[core/vm: reject contract creation if the storage is non-empty (](https://github.com/ethereum/go-ethereum/commit/c170cc0ab0a1f60adcde80d0af8e3050ee19da93)[#28912](https://github.com/ethereum/go-ethereum/pull/28912)[)](https://github.com/ethereum/go-ethereum/commit/c170cc0ab0a1f60adcde80d0af8e3050ee19da93)
[Add state reader abstraction (#29761)](https://github.com/ethereum/go-ethereum/pull/29761)
[Stateless witness prefetcher changes (#29519)](https://github.com/ethereum/go-ethereum/pull/29519)
_not follow changes with trie_prefetcher, the implemenation in bsc is very different_
[core: use finalized block as the chain freeze indicator (](https://github.com/ethereum/go-ethereum/commit/ca473b81cbe4a96cde4e8424c49b15ab304787bb)[#28683](https://github.com/ethereum/go-ethereum/pull/28683)[)](https://github.com/ethereum/go-ethereum/commit/ca473b81cbe4a96cde4e8424c49b15ab304787bb)
_in bsc, this feature only enabled with multi-database_

#### New EIPs
[core/vm: enable bls-precompiles for Prague (](https://github.com/ethereum/go-ethereum/commit/823719b9e1b72174cd8245ae9e6f6f7d7072a8d6)[#29552](https://github.com/ethereum/go-ethereum/pull/29552)[)](https://github.com/ethereum/go-ethereum/commit/823719b9e1b72174cd8245ae9e6f6f7d7072a8d6)
[EIP-2935: Serve historical block hashes from state](https://eips.ethereum.org/EIPS/eip-2935) ([#29465](https://github.com/ethereum/go-ethereum/pull/29465))

#### Clear Up
[eth, eth/downloader: remove references to LightChain, LightSync (#29711)](https://github.com/ethereum/go-ethereum/pull/29711)
[eth/filters: remove support for pending logs(#29574)](https://github.com/ethereum/go-ethereum/pull/29574)
[Drop large-contract (500MB+) deletion DoS protection from pathdb post Cancun (#28940)](https://github.com/ethereum/go-ethereum/pull/28940).
Remove totalDifficulty field from RPC, in accordance with [spec update](https://github.com/ethereum/execution-apis/pull/570), [#30386](https://github.com/ethereum/go-ethereum/pull/30386)

#### Merged but Reverted
[consensus, cmd, core, eth: remove support for non-merge mode of operation (](https://github.com/ethereum/go-ethereum/commit/f4d53133f6e4b13f0dbcfef3bc45e9650d863b73)[#29169](https://github.com/ethereum/go-ethereum/pull/29169)[)](https://github.com/ethereum/go-ethereum/commit/f4d53133f6e4b13f0dbcfef3bc45e9650d863b73)
[miner: refactor the miner, make the pending block on demand (](https://github.com/ethereum/go-ethereum/commit/d8e0807da22eb922539d15b0d5d01ccdd58b1267)[#28623](https://github.com/ethereum/go-ethereum/pull/28623)[)](https://github.com/ethereum/go-ethereum/commit/d8e0807da22eb922539d15b0d5d01ccdd58b1267)
[miner: lower default min miner tip from 1 gwei to 0.001 gwei(#29895)](https://github.com/ethereum/go-ethereum/pull/29895)
_bsc only has tip, 1 Gwei is the min value now_
[eth/downloader: purge pre-merge sync code (](https://github.com/ethereum/go-ethereum/commit/45baf21111c03d2954c81fdf828e630a8d7b05c1)[#29281](https://github.com/ethereum/go-ethereum/pull/29281)[)](https://github.com/ethereum/go-ethereum/commit/45baf21111c03d2954c81fdf828e630a8d7b05c1)
[all: remove forkchoicer and reorgNeeded (](https://github.com/ethereum/go-ethereum/commit/b0b67be0a2559073c1620555d2b6a73f825f7135)[#29179](https://github.com/ethereum/go-ethereum/pull/29179)[)](https://github.com/ethereum/go-ethereum/commit/b0b67be0a2559073c1620555d2b6a73f825f7135)

#### Others
[all: update to go version 1.23.0 (#30323)](https://github.com/ethereum/go-ethereum/pull/30323)
[Switch to using Go's native log/slog package instead of golang/exp (#29302)](https://github.com/ethereum/go-ethereum/pull/29302).
[Add the geth db inspect-history command to inspect pathdb state history (#29267)](https://github.com/ethereum/go-ethereum/pull/29267).
Improve the discovery protocol's node revalidation ([#29572](https://github.com/ethereum/go-ethereum/pull/29572), [#29864](https://github.com/ethereum/go-ethereum/pull/29864), [#29836](https://github.com/ethereum/go-ethereum/pull/29836)).
[Blobpool related flags in Geth now actually work. (#30203)](https://github.com/ethereum/go-ethereum/pull/30203)
[core/rawdb: implement in-memory freezer (#29135)](https://github.com/ethereum/go-ethereum/commit/f46c878441e2e567e8815f1e252a38ad0ffafbc2)


## v1.5.0
v1.5.0 was skipped as we forgot to pump the version when create the tag, it is replaced by v1.5.1

## v1.4.16
### BUGFIX
* [\#2736](https://github.com/bnb-chain/bsc/pull/2736) ethclient: move TransactionOpts to avoid import internal package;
* [\#2755](https://github.com/bnb-chain/bsc/pull/2755) fix: fix multi-db env
* [\#2759](https://github.com/bnb-chain/bsc/pull/2759) fix: add blobSidecars in db inspect
* [\#2764](https://github.com/bnb-chain/bsc/pull/2764) fix: add blobSidecars in db inspect

### FEATURE
* [\#2692](https://github.com/bnb-chain/bsc/pull/2692) feat: add pascal hardfork
* [\#2718](https://github.com/bnb-chain/bsc/pull/2718) feat: add Prague hardfork
* [\#2734](https://github.com/bnb-chain/bsc/pull/2734) feat: update system contract bytecodes of pascal hardfork
* [\#2737](https://github.com/bnb-chain/bsc/pull/2737) feat: modify LOCK_PERIOD_FOR_TOKEN_RECOVER to 300 seconds on BSC Testnet in pascal hardfork
* [\#2660](https://github.com/bnb-chain/bsc/pull/2660) core/txpool/legacypool: add overflowpool for txs
* [\#2754](https://github.com/bnb-chain/bsc/pull/2754) core/txpool: improve Add() logic, handle edge case

### IMPROVEMENT
* [\#2727](https://github.com/bnb-chain/bsc/pull/2727) core: clearup testflag for Bohr
* [\#2716](https://github.com/bnb-chain/bsc/pull/2716) minor Update group_prover.sage
* [\#2735](https://github.com/bnb-chain/bsc/pull/2735) concensus/parlia.go: make distribute incoming tx more independence
* [\#2742](https://github.com/bnb-chain/bsc/pull/2742) feat: remove pipecommit
* [\#2748](https://github.com/bnb-chain/bsc/pull/2748) jsutil: put all js utils in one file
* [\#2749](https://github.com/bnb-chain/bsc/pull/2749) jsutils: add tool GetKeyParameters
* [\#2756](https://github.com/bnb-chain/bsc/pull/2756) nancy: ignore github.com/golang-jwt/jwt/v4 4.5.0 in .nancy-ignore
* [\#2757](https://github.com/bnb-chain/bsc/pull/2757) util: python script to get stats of reorg
* [\#2758](https://github.com/bnb-chain/bsc/pull/2758) utils: print monikey for reorg script
* [\#2714](https://github.com/bnb-chain/bsc/pull/2714) refactor: Directly swap two variables to optimize code

## v1.4.15
### BUGFIX
* [\#2680](https://github.com/bnb-chain/bsc/pull/2680) txpool: apply miner's gasceil to txpool
* [\#2688](https://github.com/bnb-chain/bsc/pull/2688) txpool: set default GasCeil from 30M to 0
* [\#2696](https://github.com/bnb-chain/bsc/pull/2696) miner: limit block size to eth protocol msg size
* [\#2684](https://github.com/bnb-chain/bsc/pull/2684) eth: Add sidecars when available to broadcasted current block

### FEATURE
* [\#2672](https://github.com/bnb-chain/bsc/pull/2672) faucet: with mainnet balance check, 0.002BNB at least
* [\#2678](https://github.com/bnb-chain/bsc/pull/2678) beaconserver: simulated beacon api server for op-stack
* [\#2687](https://github.com/bnb-chain/bsc/pull/2687) faucet: support customized token
* [\#2698](https://github.com/bnb-chain/bsc/pull/2698) faucet: add example for custimized token
* [\#2706](https://github.com/bnb-chain/bsc/pull/2706) faucet: update DIN token faucet support

### IMPROVEMENT
* [\#2677](https://github.com/bnb-chain/bsc/pull/2677) log: add some p2p log
* [\#2679](https://github.com/bnb-chain/bsc/pull/2679) build(deps): bump actions/download-artifact in /.github/workflows
* [\#2662](https://github.com/bnb-chain/bsc/pull/2662) metrics: add some extra feature flags as node stats
* [\#2675](https://github.com/bnb-chain/bsc/pull/2675) fetcher: Sleep after marking block as done when requeuing
* [\#2695](https://github.com/bnb-chain/bsc/pull/2695) CI: nancy ignore CVE-2024-8421
* [\#2689](https://github.com/bnb-chain/bsc/pull/2689) consensus/parlia: wait more time when processing huge blocks

## v1.4.14

### BUGFIX
* [\#2643](https://github.com/bnb-chain/bsc/pull/2643)core: fix cache for receipts
* [\#2656](https://github.com/bnb-chain/bsc/pull/2656)ethclient: fix BlobSidecars api
* [\#2657](https://github.com/bnb-chain/bsc/pull/2657)fix: update prunefreezer’s offset when pruneancient and the dataset has pruned block

### FEATURE
* [\#2661](https://github.com/bnb-chain/bsc/pull/2661)config: setup Mainnet 2 hardfork date: HaberFix & Bohr

### IMPROVEMENT
* [\#2578](https://github.com/bnb-chain/bsc/pull/2578)core/systemcontracts: use vm.StateDB in UpgradeBuildInSystemContract
* [\#2649](https://github.com/bnb-chain/bsc/pull/2649)internal/debug: remove memsize
* [\#2655](https://github.com/bnb-chain/bsc/pull/2655)internal/ethapi: make GetFinalizedHeader monotonically increasing
* [\#2658](https://github.com/bnb-chain/bsc/pull/2658)core: improve readability of the fork choice logic
* [\#2665](https://github.com/bnb-chain/bsc/pull/2665)faucet: bump and resend faucet transaction if it has been pending for a while

## v1.4.13

### BUGFIX
* [\#2602](https://github.com/bnb-chain/bsc/pull/2602) fix: prune-state when specify --triesInMemory 32
* [\#2579](https://github.com/bnb-chain/bsc/pull/00025790) fix: only take non-mempool tx to calculate bid price

### FEATURE
* [\#2634](https://github.com/bnb-chain/bsc/pull/2634) config: setup Testnet Bohr hardfork date
* [\#2482](https://github.com/bnb-chain/bsc/pull/2482) BEP-341: Validators can produce consecutive blocks
* [\#2502](https://github.com/bnb-chain/bsc/pull/2502) BEP-402: Complete Missing Fields in Block Header to Generate Signature
* [\#2558](https://github.com/bnb-chain/bsc/pull/2558) BEP-404: Clear Miner History when Switching Validators Set
* [\#2605](https://github.com/bnb-chain/bsc/pull/2605) feat: add bohr upgrade contracts bytecode
* [\#2614](https://github.com/bnb-chain/bsc/pull/2614) fix: update stakehub bytecode after zero address agent issue fixed
* [\#2608](https://github.com/bnb-chain/bsc/pull/2608) consensus/parlia: modify mining time for last block in one turn
* [\#2618](https://github.com/bnb-chain/bsc/pull/2618) consensus/parlia: exclude inturn validator when calculate backoffTime
* [\#2621](https://github.com/bnb-chain/bsc/pull/2621) core: not record zero hash beacon block root with Parlia engine

### IMPROVEMENT
* [\#2589](https://github.com/bnb-chain/bsc/pull/2589) core/vote: vote before committing state and writing block
* [\#2596](https://github.com/bnb-chain/bsc/pull/2596) core: improve the network stability when double sign happens
* [\#2600](https://github.com/bnb-chain/bsc/pull/2600) core: cache block after wroten into db
* [\#2629](https://github.com/bnb-chain/bsc/pull/2629) utils: add GetTopAddr to analyse large traffic
* [\#2591](https://github.com/bnb-chain/bsc/pull/2591) consensus/parlia: add GetJustifiedNumber and GetFinalizedNumber
* [\#2611](https://github.com/bnb-chain/bsc/pull/2611) cmd/utils: add new flag OverridePassedForkTime
* [\#2603](https://github.com/bnb-chain/bsc/pull/2603) faucet: rate limit initial implementation
* [\#2622](https://github.com/bnb-chain/bsc/pull/2622) tests: fix evm-test CI
* [\#2628](https://github.com/bnb-chain/bsc/pull/2628) Makefile: use docker compose v2 instead of v1

## v1.4.12

### BUGFIX
* [\#2557](https://github.com/bnb-chain/bsc/pull/2557) fix: fix state inspect error after pruned state
* [\#2562](https://github.com/bnb-chain/bsc/pull/2562) fix: delete unexpected block
* [\#2566](https://github.com/bnb-chain/bsc/pull/2566) core: avoid to cache block before wroten into db
* [\#2567](https://github.com/bnb-chain/bsc/pull/2567) fix: fix statedb copy
* [\#2574](https://github.com/bnb-chain/bsc/pull/2574) core: adapt highestVerifiedHeader to FastFinality
* [\#2542](https://github.com/bnb-chain/bsc/pull/2542) fix: pruneancient freeze from the previous position when the first time
* [\#2564](https://github.com/bnb-chain/bsc/pull/2564) fix: the bug of blobsidecars and downloader with multi-database
* [\#2582](https://github.com/bnb-chain/bsc/pull/2582) fix: remove delete and dangling side chains in prunefreezer

### FEATURE
* [\#2513](https://github.com/bnb-chain/bsc/pull/2513) cmd/jsutils: add a tool to get performance between a range of blocks
* [\#2569](https://github.com/bnb-chain/bsc/pull/2569) cmd/jsutils: add a tool to get slash count
* [\#2583](https://github.com/bnb-chain/bsc/pull/2583) cmd/jsutill: add log about validator name

### IMPROVEMENT
* [\#2546](https://github.com/bnb-chain/bsc/pull/2546) go.mod: update missing dependency
* [\#2559](https://github.com/bnb-chain/bsc/pull/2559) nancy: ignore go-retryablehttp@v0.7.4 in .nancy-ignore
* [\#2556](https://github.com/bnb-chain/bsc/pull/2556) chore: update greenfield cometbft version
* [\#2561](https://github.com/bnb-chain/bsc/pull/2561) tests: fix unstable test
* [\#2572](https://github.com/bnb-chain/bsc/pull/2572) core: clearup testflag for Cancun and Haber
* [\#2573](https://github.com/bnb-chain/bsc/pull/2573) cmd/utils: support use NetworkId to distinguish chapel when do syncing
* [\#2538](https://github.com/bnb-chain/bsc/pull/2538) feat: enhance bid comparison and reply bidding results && detail logs
* [\#2568](https://github.com/bnb-chain/bsc/pull/2568) core/vote: not vote if too late for next in turn validator
* [\#2576](https://github.com/bnb-chain/bsc/pull/2576) miner/worker: broadcast block immediately once sealed
* [\#2580](https://github.com/bnb-chain/bsc/pull/2580) freezer: Opt freezer env checking

## v1.4.11

### BUGFIX
* [\#2534](https://github.com/bnb-chain/bsc/pull/2534) fix: nil pointer when clear simulating bid
* [\#2535](https://github.com/bnb-chain/bsc/pull/2535) upgrade: add HaberFix hardfork

## v1.4.10
### FEATURE
NA
>>>>>>> bsc/v1.5.12

## v1.0.11

Improvements
* [#1](https://github.com/coredao-org/core-genesis-contract/commit/abcc6f9c7323c1202dd1f91a8637fcc00401a4ab) Implemented BTC/CORE dual staking
* [#2](https://github.com/coredao-org/core-genesis-contract/commit/abcc6f9c7323c1202dd1f91a8637fcc00401a4ab) Implemented lstBTC
* [#3](https://github.com/coredao-org/core-chain/commit/3f35806416f50f534ad8d1f8a7eccec2582e7b16) Merged versions up to v1.3.9 from Binance smart chain (originally planned as v1.0.10)

## v1.0.9

Improvements
* [#1](https://github.com/coredao-org/core-chain/commit/96abe9d1c72baac567020a20f4fdb3538bef32f5) Add query limit to defend DDOS attack.
* [#2](https://github.com/coredao-org/core-chain/commit/af906cc8e286d6c9487fddc54b06b9e5e98f1572) Moving the response sending there allows tracking all peer goroutines
* [#3](https://github.com/coredao-org/core-chain/pull/32/commits/5ebb5fc8e29f225194a603fc753a3f2f006c178a) implement ENR node in p2p/discover
* [#4] Enlarge the default block gas limit from 40m to 50m.

## v1.0.8

Same changes with 1.0.7, but this version is for mainnet.

## v1.0.7

Improvements
* [#1](https://github.com/coredao-org/core-genesis-contract/commit/fbb4a12b0e7d7239fff0eaf15f37edfe762e987e) Enables self custody BTC staking on Core blockchain.

## v1.0.6

Same changes with 1.0.5, but this version is for mainnet.

## v1.0.5

Improvements
* [#1](https://github.com/coredao-org/core-genesis-contract/commit/8b8442e8917715734b38018b76f77431e57990d7) support to verify normal bitcoin transaction base on the system contracts named BtcLightClient

## v1.0.4

Same changes with 1.0.3, but this version is for mainnet.

## v1.0.3

Improvements
* [#1](https://github.com/coredao-org/core-genesis-contract/commit/220efb36b89ca354686e2fff6dfae9ca920dea39) Improve staking experiences
* [#2](https://github.com/coredao-org/core-genesis-contract/commit/a5b6f29b3c979a09a06ff07aacdeeda119bd53e2) Relayer anti MEV
* [#3](https://github.com/coredao-org/core-chain/compare/branch_v1.0.2...branch_v1.0.3) BEP-172: Network Stability Enhancement On Slash Occur

BUGFIX
* [#4](https://github.com/coredao-org/core-genesis-contract/commit/5656c27433069470a011b89118b8f77e3fc6abab) Gnosis gas issue
* [#5](https://github.com/coredao-org/core-genesis-contract/commit/6526ca8389dc11c6628e0b7d1f3fba73528f58b7) Potential turnround failure caused by relayer offline
* [#6](https://github.com/coredao-org/core-genesis-contract/commit/62a81d5ac686d04b24fcd05920ef9bff5cea78bc) Enable validator set to accept CORE transfer
* [#7](https://github.com/coredao-org/core-genesis-contract/commit/b7f5427aa7e78a12cee3e0add52300c832b10289) Fix bug that modify commissions beyond the limited range

## v1.0.2

IMPROVEMENT
* [\#2](https://github.com/coredao-org/core-chain/commit/33d8d200aa300cea80bd4b91e7df6a81af481f1d) merge bsc v1.1.19

BUGFIX
* [\#1](https://github.com/coredao-org/core-chain/commit/ed4094e96e0d009dac9ff13473b022be430f9232) core/txpool: implement additional DoS defenses

## v1.0.1
* mainnet released.

FEATURES
* [\#1]() upgrade hashpower mapping


## v1.0.0
FEATURES
* [\#1]() implement satoshi plus consensus and bitcoin light client

## Initial fork

* forked from bsc [v1.1.8](https://github.com/bnb-chain/bsc/tree/v1.1.8)
