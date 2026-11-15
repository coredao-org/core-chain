# Core archive validation

The JSON reports identify the exact candidate binary and reference client.
`manifest.json` pins the initially validated Go source commit, both upstream bases, the
canonical RLP input hash, archive settings, and validation limits. Reference RPC
addresses and credentials are not recorded.

- Imported mainnet blocks 1–512 (532 transactions) into a fresh path archive and
  restarted it before comparison. Normal block import validates every state
  root, receipt root, gas total and bloom.
- `rpc-comparison.json`: 102 comparisons at blocks 1, 2, 128, 256 and 512,
  including historical ValidatorSet contract calls, gas estimation, state reads,
  receipts, logs, native tracers and a JavaScript tracer. Only the additional
  `blockTimestamp` field is ignored; seven candidate log timestamps were also
  checked against their block timestamps.
- `modern-replay.json`: five transactions at blocks 19,537,201; 23,000,000;
  30,000,000; 40,000,011; and 40,308,868. Each passed `callTracer` and
  `prestateTracer` comparisons, and each replay matched the reference node's
  original transaction trace. The report records hashes of the captured input
  and normalized output.
- `berlin-boundary.json`: source `86bcb14892c3ca01e4771e2fee5de62003fdd0cd`
  passed full-sync import across Berlin and 24 official-client comparisons at
  blocks 19,537,199–19,537,201, including complete receipts, block call/prestate
  traces, historical contract calls and state reads. It fixes the system-transaction
  access-list panic at Berlin while preserving Core's cold-access gas rules before
  Hermes. The regression reproduces the panic before the fix and checks gas costs
  before Berlin, at Berlin, and at Hermes; Satoshi, StateDB and EVM suites pass.
- `full-history-audit.json` and `full-history-plan.json`: after full mainnet sync,
  900 historical blocks passed 25,409 comparisons, covering 8,380 transactions and
  12,367 receipt logs. The sample combines deterministic stratified random blocks,
  dense early/recent ranges, and all 11 configured fork boundaries. Both native
  block tracers ran at 125 heights, with additional transaction/call traces,
  historical state, contract calls, receipts, raw blocks and logs compared.
  A separate deterministic 10,000-block sample and 64 filtered historical log
  windows of 5,000 blocks each added 10,064 passing comparisons. In total,
  35,473 comparisons passed across 10,899 distinct sampled block heights.
- `filtermaps.json` and `filtermaps-rpc.json`: qualification of the subsequent
  Geth v1.16.9 log-index integration, with an exact source commit and binary hash.
  The 900 historical sample blocks contain 12,367 logs; their raw blocks,
  receipts and block-hash log queries match 2,700 recorded official-client
  results. Indexed/reopened/limited/disabled searches passed 14,104 comparisons.
  Native disk restart, a deep rewind from 512 to 256, canonical reimport and
  disabled-index fallback passed 389 RPC/tracing checks and 20 strict log-range
  comparisons. Sixteen later transactions across all 11 fork groups passed 32
  sparse replay comparisons, each also matching its original transaction trace.
  These initial fixture checks did not include another complete mainnet sync or
  storage benchmark. Fixture samples use consecutive positions rather than the full
  mainnet layout; the report distinguishes that from native block-range tests.
  The complete `./tests` conformance suite also passed without short mode using
  execution-spec-tests v5.1.0 and all storage configurations. Its two exclusions
  match Geth v1.16.9's missing-system-contract deployment fixture exclusions and
  are listed in the report.
- `mainnet-filtermaps.json`: final executable source
  `fff0d506f76b4557a5c977c3ac3c0b7b0deed858`, with all seven CI checks passing.
  A fresh canonical 512-block import and 102 RPC comparisons passed. The binary
  then built the complete mainnet log index over a consistent copy of the
  previously full-synced archive: blocks 0–40,451,575, followed by a clean restart
  that reloaded through 40,451,617 with the head indexed. Indexed queries produced
  filtermaps row reads. The final binary matched 35,473 recorded official-client
  responses, including 500 tracing comparisons, across the historical plan.
  After full indexing and restart, 1,657 filter queries at 94 historical heights
  matched the previously qualified bloom-index archive; 150 official-golden
  replay checks, 41 recent state/tracing comparisons and WebSocket subscriptions
  also passed. Only `blockTimestamp` is normalized, with separate header checks.
  Whole-datadir allocation was about 368.7 GiB, versus 363.8 GiB for the earlier
  path archive; this includes compaction and chain growth. This run did not repeat
  the final binary's state sync from genesis. The report distinguishes direct
  official-response replay, bloom-reference comparisons and index lifecycle
  evidence, and records hashes of local transcripts without embedding endpoints.

Modern replays use touched prestate and block overrides. They do not validate
complete modern block state roots, consensus or state outside the captured
accounts. Full mainnet sync has since completed, with normal canonical state-root
and receipt-root validation. Historical query comparisons remain sampled; disk
observations are not a controlled benchmark. Historical Merkle proofs and validator
operation are outside this qualification.

See [reproduction instructions](../README.archive.md). Local build and regression
checks use Go 1.24 with `GOMAXPROCS=2`, `go test -p=1 -parallel=2`, and linter
concurrency 2.

Local checks passed: geth build, compilation of all test packages, the complete
Core blockchain test suite, VM/state/rawdb/system-contract/Satoshi-consensus
tests, native/JS/logger tracer tests, generated-code and module-tidy checks, and
golangci-lint 2.4.0 (zero issues). Exact commands are in the manifest.
