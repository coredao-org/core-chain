#!/usr/bin/env python3
"""Compare a Core RPC/archive candidate with an official node, sequentially."""

import argparse
import hashlib
import json
import pathlib
import urllib.request


def rpc(url, method, params):
    request = urllib.request.Request(
        url,
        data=json.dumps(dict(jsonrpc="2.0", id=1, method=method, params=params)).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=120) as response:
        value = json.load(response)
    if "error" in value:
        raise RuntimeError(f"{method}: {value['error']}")
    return value["result"]


def comparable(value):
    # Core v1.0.27 predates Geth 1.16's additional log timestamp field.
    if isinstance(value, dict):
        return {key: comparable(item) for key, item in value.items() if key != "blockTimestamp"}
    if isinstance(value, list):
        return [comparable(item) for item in value]
    return value


def replay_params(reference, tx_hash):
    """Capture touched prestate and original context for a transaction replay.

    Both nodes execute against block 512 plus identical state/context overrides.
    This checks fork-specific EVM execution, not the original block's state root.
    Contracts using BLOCKHASH need a full replay instead of this sparse fixture.
    """
    tx = rpc(reference, "eth_getTransactionByHash", [tx_hash])
    block = rpc(reference, "eth_getBlockByHash", [tx["blockHash"], False])
    prestate = rpc(reference, "debug_traceTransaction", [tx_hash, dict(tracer="prestateTracer", timeout="60s")])
    accounts = {}
    for address, account in prestate.items():
        accounts[address] = dict(
            balance=account.get("balance", "0x0"),
            nonce=hex(account.get("nonce", 0)),
            code=account.get("code", "0x"),
            state=account.get("storage", {}),
        )
    call = {key: tx[key] for key in ("from", "to", "gas", "gasPrice", "value", "nonce")}
    call["data"] = tx["input"]
    context = dict(
        number=block["number"], time=block["timestamp"], difficulty=block["difficulty"],
        gasLimit=block["gasLimit"], feeRecipient=block["miner"], baseFeePerGas=block.get("baseFeePerGas", "0x0"),
    )
    if block.get("excessBlobGas") is not None:
        # Core's Cancun/Prague schedules use the upstream exponential blob fee.
        denominator = 5007716 if int(block["timestamp"], 16) >= 1764057600 else 3338477
        excess, accumulator, result, iteration = int(block["excessBlobGas"], 16), denominator, 0, 1
        while accumulator:
            result += accumulator
            accumulator = accumulator * excess // (denominator * iteration)
            iteration += 1
        context["blobBaseFee"] = hex(result // denominator)
    return [call, "0x200", dict(stateOverrides=accounts, blockOverrides=context, timeout="60s")]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reference", required=True)
    parser.add_argument("--candidate")
    parser.add_argument("--blocks", default="1,2,128,256")
    parser.add_argument("--export-blocks", type=int, help="Export blocks 1..N for offline import")
    parser.add_argument("--replay-transactions", nargs="+", help="Compare sparse-prestate transaction replays")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.export_blocks:
        with args.output.open("wb") as output:
            for number in range(1, args.export_blocks + 1):
                raw = rpc(args.reference, "debug_getRawBlock", [hex(number)])
                output.write(bytes.fromhex(raw.removeprefix("0x")))
        print(f"Exported {args.export_blocks} canonical blocks to {args.output}")
        return
    if not args.candidate:
        parser.error("--candidate is required for comparison")
    report = {
        "referenceVersion": rpc(args.reference, "web3_clientVersion", []),
        "candidateVersion": rpc(args.candidate, "web3_clientVersion", []),
        "allowedAdditionalFields": ["blockTimestamp"],
        "checks": [],
    }

    def compare(method, params, label=None):
        expected = rpc(args.reference, method, params)
        actual = rpc(args.candidate, method, params)
        if expected is None or actual is None:
            raise AssertionError(f"{method} returned null: {params}")
        if comparable(actual) != comparable(expected):
            args.output.with_suffix(".mismatch.json").write_text(
                json.dumps(dict(method=method, params=params, expected=expected, actual=actual), indent=2)
            )
            raise AssertionError(f"{method} differs: {params}")
        check = dict(method=method, params=params if label is None else label)
        check["sha256"] = hashlib.sha256(json.dumps(comparable(expected), sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        check["inputSha256"] = hashlib.sha256(json.dumps(params, sort_keys=True, separators=(",", ":")).encode()).hexdigest()
        report["checks"].append(check)
        print(f"PASS {method} {params if label is None else label}", flush=True)
        return expected

    for tx_hash in args.replay_transactions or []:
        params = replay_params(args.reference, tx_hash)
        for tracer in ("callTracer", "prestateTracer"):
            params[2]["tracer"] = tracer
            params[2]["tracerConfig"] = dict(withLog=True) if tracer == "callTracer" else dict(diffMode=True)
            result = compare("debug_traceCall", params, dict(transaction=tx_hash, number=params[2]["blockOverrides"]["number"], tracer=tracer))
            original = rpc(args.reference, "debug_traceTransaction", [tx_hash, dict(
                tracer=tracer, tracerConfig=params[2]["tracerConfig"], timeout="60s",
            )])
            if comparable(result) != comparable(original):
                raise AssertionError(f"Sparse replay differs from original transaction: {tx_hash} ({tracer})")
            report["checks"][-1]["matchesOriginalTransaction"] = True

    block_numbers = [] if args.replay_transactions else (int(value, 0) for value in args.blocks.split(","))
    for number in block_numbers:
        tag = hex(number)
        block = compare("eth_getBlockByNumber", [tag, False])
        compare("eth_getBlockByHash", [block["hash"], True])
        for address in (block["miner"], "0x0000000000000000000000000000000000001000"):
            compare("eth_getBalance", [address, tag])
            compare("eth_getTransactionCount", [address, tag])
            compare("eth_getCode", [address, tag])
            compare("eth_getStorageAt", [address, "0x0", tag])
        compare("eth_getLogs", [dict(blockHash=block["hash"])])
        # Query ValidatorSet.getValidators() to exercise historical contract storage.
        call = dict(to="0x0000000000000000000000000000000000001000", data="0xb7ab4db5", gas="0x989680")
        compare("eth_call", [call, tag])
        compare("eth_estimateGas", [call, tag])
        compare("debug_traceCall", [call, tag, dict(tracer="callTracer", timeout="60s")])
        compare("debug_traceBlockByNumber", [tag, dict(tracer="callTracer", timeout="60s")])
        compare("debug_traceBlockByNumber", [tag, dict(tracer="prestateTracer", timeout="60s")])
        # Exercise the parallel JS tracer with at most two transaction workers.
        if len(block["transactions"]) == 2:
            tracer = "{steps:0,step:function(log,db){this.steps++;},fault:function(log,db){},result:function(ctx,db){return this.steps;}}"
            compare("debug_traceBlockByNumber", [tag, dict(tracer=tracer, timeout="60s")])
        for tx_hash in dict.fromkeys(block["transactions"][:1] + block["transactions"][-1:]):
            compare("eth_getTransactionByHash", [tx_hash])
            compare("eth_getTransactionReceipt", [tx_hash])
            compare("debug_traceTransaction", [tx_hash, dict(tracer="callTracer", timeout="60s")])
    report["passed"] = len(report["checks"])
    args.output.write_text(json.dumps(report, indent=2) + "\n")
    print(f"All {report['passed']} checks passed; report: {args.output}")


if __name__ == "__main__":
    main()
