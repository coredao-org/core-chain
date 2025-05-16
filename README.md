## Core chain

Core is an evolution of the Geth codebase. We leveraged the improvements made by the BSC team to add greater throughput and cheaper transactions by way of hard fork. Nevertheless, we differ from BSC in many ways. One preeminent difference is that Core is based on Satoshi Plus Consensus which relies on Proof of Work (PoW) alongside Delegated Proof of Stake (DPoS). With these modifications, we’re able to remain decentralized without the performance tradeoffs seen in traditional PoW consensus systems. Additionally, with our hybrid score based off of both delegated Bitcoin hash power and delegated stake, we’ve created a fluid market for validators and rewards that anyone can participate in.

[![API Reference](
https://camo.githubusercontent.com/915b7be44ada53c290eb157634330494ebe3e30a/68747470733a2f2f676f646f632e6f72672f6769746875622e636f6d2f676f6c616e672f6764646f3f7374617475732e737667
)](https://pkg.go.dev/github.com/ethereum/go-ethereum?tab=doc)
<<<<<<< HEAD
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.com/invite/coredaoofficial)

More details in [White Paper](https://whitepaper.coredao.org/).
=======
[![Build Test](https://github.com/bnb-chain/bsc/actions/workflows/build-test.yml/badge.svg)](https://github.com/bnb-chain/bsc/actions)
[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://discord.gg/z2VpC455eU)

But from that baseline of EVM compatible, BNB Smart Chain introduces a system of 21 validators with Proof of Staked Authority (PoSA) consensus that can support short block time and lower fees. The most bonded validator candidates of staking will become validators and produce blocks. The double-sign detection and other slashing logic guarantee security, stability, and chain finality.

**The BNB Smart Chain** will be:

- **A self-sovereign blockchain**: Provides security and safety with elected validators.
- **EVM-compatible**: Supports all the existing Ethereum tooling along with faster finality and cheaper transaction fees.
- **Distributed with on-chain governance**: Proof of Staked Authority brings in decentralization and community participants. As the native token, BNB will serve as both the gas of smart contract execution and tokens for staking.

More details in [White Paper](https://github.com/bnb-chain/whitepaper/blob/master/WHITEPAPER.md).
>>>>>>> bsc/v1.5.12

## Key features

### Satoshi Plus Consensus

Satoshi Plus consensus combines both Proof of Work (PoW) and Delegated Proof of Stake (DPoS) to leverage the strengths of each while simultaneously ameliorating their respective shortcomings. Specifically, we leverage Bitcoin’s decentralized computing power, DPoS and unique validator election mechanisms to ensure scalability, and the resulting, multifaceted network’s security.

### Bitcoin Light Client

To achieve the cross-chain communication from Bitcoin Network to Core chain, we introduced an on-chain light client verification algorithm. It contains two parts:

<<<<<<< HEAD
1. Stateless precompiled contract to do bitcoin header verification, coinbase transaction verification and Merkle Proof verification.
2. Stateful solidity contract to store bitcoin block hashes and headers.

=======
1. Blocks are produced by a limited set of validators.
2. Validators take turns to produce blocks in a PoA manner, similar to Ethereum's Clique consensus engine.
3. Validator set are elected in and out based on a staking based governance on BNB Smart Chain.
4. Parlia consensus engine will interact with a set of [system contracts](https://docs.bnbchain.org/bnb-smart-chain/staking/overview/#system-contracts) to achieve liveness slash, revenue distributing and validator set renewing func.

## Native Token

BNB will run on BNB Smart Chain in the same way as ETH runs on Ethereum so that it remains as `native token` for BSC. This means,
BNB will be used to:

1. pay `gas` to deploy or invoke Smart Contract on BSC
>>>>>>> bsc/v1.5.12

## Building the source

Many of the below are the same as or similar to go-ethereum.

For prerequisites and detailed build instructions please read the [Installation Instructions](https://geth.ethereum.org/docs/getting-started/installing-geth).

<<<<<<< HEAD
Building `geth` requires both a Go (version 1.14 or later) and a C compiler. You can install
=======
Building `geth` requires both a Go (version 1.22 or later) and a C compiler (GCC 5 or higher). You can install
>>>>>>> bsc/v1.5.12
them using your favourite package manager. Once the dependencies are installed, run

```shell
make geth
```

or, to build the full suite of utilities:

```shell
make all
```

## Executables

The Core project comes with several wrappers/executables found in the `cmd`
directory.

|    Command    | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| :-----------: | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
|  **`geth`**   | Core chain client binary. It is the entry point into the Core chain network (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It has the same and more RPC and other interface as go-ethereum and can be used by other processes as a gateway into the Core chain network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `geth --help` and the [CLI page](https://geth.ethereum.org/docs/fundamentals/command-line-options) for command line options.          |
|   `clef`      | Stand-alone signing tool, which can be used as a backend signer for `geth`.  |
|   `devp2p`    | Utilities to interact with nodes on the networking layer, without running a full blockchain. |
|   `abigen`    | Source code generator to convert Ethereum contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain [Ethereum contract ABIs](https://docs.soliditylang.org/en/develop/abi-spec.html) with expanded functionality if the contract bytecode is also available. However, it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://geth.ethereum.org/docs/developers/dapp-developer/native-bindings) page for details. |
|  `bootnode`   | Stripped down version of our Ethereum client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks.                                                                                                                                                                                                                                                                 |
|     `evm`     | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`).                                                                                                                                                                                                                                                                     |
|   `rlpdump`   | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://ethereum.org/en/developers/docs/data-structures-and-encoding/rlp/)) dumps (data encoding used by the Ethereum protocol both network as well as consensus wise) to user-friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`).                                                                                                                                                                                                                                 |

## Running `geth`

Going through all the possible command line flags is out of scope here (please consult our
[CLI Wiki page](https://geth.ethereum.org/docs/fundamentals/command-line-options)),
but we've enumerated a few common parameter combos to get you up to speed quickly
on how you can run your own `geth` instance.

### Hardware Requirements

The hardware must meet certain requirements to run a full node. Please check [How to run a core fullnode](https://docs.coredao.org/docs/Node/Full-Node/on-mainnet#hardware-specifications-for-full-nodes-on-core-mainnet).

```shell
$ geth console
```

This command will:

 * Start `geth` in fast sync mode (default, can be changed with the `--syncmode` flag),
   causing it to download more data in exchange for avoiding processing the entire history
   of the Core chain network, which is very CPU intensive.
 * Start up `geth`'s built-in interactive [JavaScript console](https://geth.ethereum.org/docs/interacting-with-geth/javascript-console),
   (via the trailing `console` subcommand) through which you can interact using [`web3` methods](https://web3js.readthedocs.io/en/latest/) 
   (note: the `web3` version bundled within `geth` is very old, and not up to date with official docs),
   as well as `geth`'s own [management APIs](https://geth.ethereum.org/docs/interacting-with-geth/rpc).
   This tool is optional and if you leave it out you can always attach to an already running
   `geth` instance with `geth attach`.

### Run Full node on the Core network

<<<<<<< HEAD
Steps:

1. Download the binary, config and genesis files from [latest release](https://github.com/coredao-org/core-chain/releases), or compile the binary by `make geth`. 
2. Init genesis state: `./geth --datadir node init genesis.json`.
3. Start your fullnode: `./geth --config ./config.toml --datadir ./node`.
4. Or start a validator node: `./geth --config ./config.toml --datadir ./node -unlock ${validatorAddr} --mine --allow-insecure-unlock`. The ${validatorAddr} is the wallet account address of your running validator node. 

More details about [running a node](https://docs.coredao.org/docs/Node/validator/running-validator) and [becoming a validator](https://docs.coredao.org/docs/Node/validator/validator-register).

*Note: Although there are some internal protective measures to prevent transactions from
crossing over between the main network and test network, you should make sure to always
use separate accounts for play-money and real-money. Unless you manually move
=======
#### 4. Start a full node
```shell
## It will run with Path-Base Storage Scheme by default and enable inline state prune, keeping the latest 90000 blocks' history state.
./geth --config ./config.toml --datadir ./node  --cache 8000 --rpc.allow-unprotected-txs --history.transactions 0

## It is recommend to run fullnode with `--tries-verify-mode none` if you want high performance and care little about state consistency.
./geth --config ./config.toml --datadir ./node  --cache 8000 --rpc.allow-unprotected-txs --history.transactions 0 --tries-verify-mode none
```

#### 5. Monitor node status

Monitor the log from **./node/bsc.log** by default. When the node has started syncing, should be able to see the following output:
```shell
t=2022-09-08T13:00:27+0000 lvl=info msg="Imported new chain segment"             blocks=1    txs=177   mgas=17.317   elapsed=31.131ms    mgasps=556.259  number=21,153,429 hash=0x42e6b54ba7106387f0650defc62c9ace3160b427702dab7bd1c5abb83a32d8db dirty="0.00 B"
t=2022-09-08T13:00:29+0000 lvl=info msg="Imported new chain segment"             blocks=1    txs=251   mgas=39.638   elapsed=68.827ms    mgasps=575.900  number=21,153,430 hash=0xa3397b273b31b013e43487689782f20c03f47525b4cd4107c1715af45a88796e dirty="0.00 B"
t=2022-09-08T13:00:33+0000 lvl=info msg="Imported new chain segment"             blocks=1    txs=197   mgas=19.364   elapsed=34.663ms    mgasps=558.632  number=21,153,431 hash=0x0c7872b698f28cb5c36a8a3e1e315b1d31bda6109b15467a9735a12380e2ad14 dirty="0.00 B"
```

#### 6. Interact with fullnode
Start up `geth`'s built-in interactive [JavaScript console](https://geth.ethereum.org/docs/interface/javascript-console),
(via the trailing `console` subcommand) through which you can interact using [`web3` methods](https://web3js.readthedocs.io/en/) 
(note: the `web3` version bundled within `geth` is very old, and not up to date with official docs),
as well as `geth`'s own [management APIs](https://geth.ethereum.org/docs/rpc/server).
This tool is optional and if you leave it out you can always attach to an already running
`geth` instance with `geth attach`.

#### 7. More

More details about [running a node](https://docs.bnbchain.org/bnb-smart-chain/developers/node_operators/full_node/) and [becoming a validator](https://docs.bnbchain.org/bnb-smart-chain/validator/create-val/)

*Note: Although some internal protective measures prevent transactions from
crossing over between the main network and test network, you should always
use separate accounts for play and real money. Unless you manually move
>>>>>>> bsc/v1.5.12
accounts, `geth` will by default correctly separate the two networks and will not make any
accounts available between them.*

### Configuration

As an alternative to passing the numerous flags to the `geth` binary, you can also pass a
configuration file via:

```shell
$ geth --config /path/to/your_config.toml
```

To get an idea how the file should look like you can use the `dumpconfig` subcommand to
export your existing configuration:

```shell
$ geth --your-favourite-flags dumpconfig
```

### Programmatically interfacing `geth` nodes

As a developer, sooner rather than later you'll want to start interacting with `geth` and the
Core chain network via your own programs and not manually through the console. To aid
this, `geth` has built-in support for a JSON-RPC based APIs ([standard APIs](https://eth.wiki/json-rpc/API)
and [`geth` specific APIs](https://geth.ethereum.org/docs/interacting-with-geth/rpc)).
These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based
platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `geth`,
whereas the HTTP and WS interfaces need to manually be enabled and only expose a
subset of APIs due to security reasons. These can be turned on/off and configured as
you'd expect.

HTTP based JSON-RPC API options:

  * `--http` Enable the HTTP-RPC server
  * `--http.addr` HTTP-RPC server listening interface (default: `localhost`)
<<<<<<< HEAD
  * `--http.port` HTTP-RPC server listening port (default: `8575`)
  * `--http.api` API's offered over the HTTP-RPC interface (default: `eth,net,web3`)
  * `--http.corsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--ws.addr` WS-RPC server listening interface (default: `localhost`)
  * `--ws.port` WS-RPC server listening port (default: `8576`)
  * `--ws.api` API's offered over the WS-RPC interface (default: `eth,net,web3`)
  * `--ws.origins` Origins from which to accept websockets requests
  * `--ipcdisable` Disable the IPC-RPC server
  * `--ipcapi` API's offered over the IPC-RPC interface (default: `admin,debug,eth,miner,net,personal,shh,txpool,web3`)
=======
  * `--http.port` HTTP-RPC server listening port (default: `8545`)
  * `--http.api` API's offered over the HTTP-RPC interface (default: `eth,net,web3`)
  * `--http.corsdomain` Comma separated list of domains from which to accept cross-origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--ws.addr` WS-RPC server listening interface (default: `localhost`)
  * `--ws.port` WS-RPC server listening port (default: `8546`)
  * `--ws.api` API's offered over the WS-RPC interface (default: `eth,net,web3`)
  * `--ws.origins` Origins from which to accept WebSocket requests
  * `--ipcdisable` Disable the IPC-RPC server
>>>>>>> bsc/v1.5.12
  * `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to
connect via HTTP, WS or IPC to a `geth` node configured with the above flags and you'll
need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You
can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based
transport before doing so! Hackers on the internet are actively trying to subvert
Core chain nodes with exposed APIs! Further, all browser tabs can access locally
running web servers, so malicious web pages could try to subvert locally available
APIs!**

<<<<<<< HEAD
=======
### Operating a private network
- [BSC-Deploy](https://github.com/bnb-chain/node-deploy/): deploy tool for setting up BNB Smart Chain.

## Running a bootnode

Bootnodes are super-lightweight nodes that are not behind a NAT and are running just discovery protocol. When you start up a node it should log your enode, which is a public identifier that others can use to connect to your node. 

First the bootnode requires a key, which can be created with the following command, which will save a key to boot.key:

```
bootnode -genkey boot.key
```

This key can then be used to generate a bootnode as follows:

```
bootnode -nodekey boot.key -addr :30311 -network bsc
```

The choice of port passed to -addr is arbitrary. 
The bootnode command returns the following logs to the terminal, confirming that it is running:

```
enode://3063d1c9e1b824cfbb7c7b6abafa34faec6bb4e7e06941d218d760acdd7963b274278c5c3e63914bd6d1b58504c59ec5522c56f883baceb8538674b92da48a96@127.0.0.1:0?discport=30311
Note: you're using cmd/bootnode, a developer tool.
We recommend using a regular node as bootstrap node for production deployments.
INFO [08-21|11:11:30.687] New local node record                    seq=1,692,616,290,684 id=2c9af1742f8f85ce ip=<nil> udp=0 tcp=0
INFO [08-21|12:11:30.753] New local node record                    seq=1,692,616,290,685 id=2c9af1742f8f85ce ip=54.217.128.118 udp=30311 tcp=0
INFO [09-01|02:46:26.234] New local node record                    seq=1,692,616,290,686 id=2c9af1742f8f85ce ip=34.250.32.100  udp=30311 tcp=0
```

>>>>>>> bsc/v1.5.12
## Contribution

Thank you for considering to help out with the source code! We welcome contributions
from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to Core, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit
more complex changes though, please check up with the core devs first on [our discord channel](https://discord.com/invite/coredaoofficial)
to ensure those changes are in line with the general philosophy of the project and/or get
some early feedback which can make both your efforts much lighter as well as our review
and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
   guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
   guidelines.
 * Pull requests need to be based on and opened against the `master` branch.
 * Commit messages should be prefixed with the package(s) they modify.
   * E.g. "eth, rpc: make trace configs optional"

Please see the [Developers' Guide](https://geth.ethereum.org/docs/developers)
for more details on configuring your environment, managing project dependencies, and
testing procedures.

## License

The core library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the `COPYING.LESSER` file.

The core binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also
included in our repository in the `COPYING` file.
