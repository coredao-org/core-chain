# Changelog

## v1.0.18

Improvements
* Remove deprecated personal API from geth

## v1.0.17

Same changes with 1.0.16, but this version is for mainnet.

## v1.0.16

Improvements
* [#1](https://github.com/coredao-org/core-genesis-contract/commit/abcc6f9c7323c1202dd1f91a8637fcc00401a4ab) Enable contract-based Rev+ share monetization and dynamic reward configuration
* [#2](https://github.com/coredao-org/core-chain/pull/54) Add live chain tracing with hooks
* [#3](https://github.com/coredao-org/core-chain/commit/cab8fb448ebd97cc4b14c09dfeff9bc5fda370aa) Merged versions up to v1.4.10 from Binance smart chain

## v1.0.15

New release for Core MainNet which contains a high security fix by the Geth team.

## v1.0.14

Same changes with 1.0.13, but this version is for mainnet.

## v1.0.13

Improvements
* Improved dual staking experiences
* Added docker support
* Bugfixes and other improvements

## v1.0.12

Same changes with 1.0.11, but this version is for mainnet.

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
