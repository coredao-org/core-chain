#!/usr/bin/env python3
"""Capture canonical Core blocks and receipts for offline log-index validation."""

import argparse
import datetime
import hashlib
import json
import pathlib
import time
import urllib.request


def comparable(value):
    if isinstance(value, dict):
        return {key: comparable(item) for key, item in value.items() if key != "blockTimestamp"}
    if isinstance(value, list):
        return [comparable(item) for item in value]
    return value


def digest(value):
    return hashlib.sha256(json.dumps(comparable(value), sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--reference", required=True)
    parser.add_argument("--plan", type=pathlib.Path, default=pathlib.Path("validation/full-history-plan.json"))
    parser.add_argument("--golden", type=pathlib.Path, help="Previously recorded official-client checks.jsonl")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    args = parser.parse_args()
    plan = json.loads(args.plan.read_text())
    heights = sorted({0, *plan["heights"]})
    golden = {}
    if args.golden:
        for line in args.golden.read_text().splitlines():
            check = json.loads(line)
            if check["method"] == "eth_getLogs" and "blockHash" in check["params"][0]:
                golden[int(check["label"])] = check
        missing = set(heights) - {0} - golden.keys()
        if missing:
            raise ValueError(f"Missing official log comparisons for {len(missing)} heights")

    blocks = []
    verified = 0
    for offset in range(0, len(heights), 2):
        selected = heights[offset:offset + 2]
        requests = []
        for height in selected:
            criteria = golden[height]["params"][0] if height in golden else {"fromBlock": hex(height), "toBlock": hex(height)}
            for method, params in (
                ("debug_getRawBlock", [hex(height)]),
                ("eth_getBlockReceipts", [hex(height)]),
                ("eth_getLogs", [criteria]),
            ):
                requests.append(dict(jsonrpc="2.0", id=len(requests), method=method, params=params))
        request = urllib.request.Request(args.reference, data=json.dumps(requests).encode(), headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(request, timeout=120) as response:
            responses = {item["id"]: item for item in json.load(response)}
        for i, height in enumerate(selected):
            values = []
            for j in range(3):
                result = responses[i * 3 + j]
                if "error" in result or result.get("result") is None:
                    raise ValueError(f"Block {height}: {requests[i * 3 + j]['method']} failed")
                values.append(result["result"])
            raw, receipts, logs = values
            log_hash = digest(logs)
            if height in golden:
                if log_hash != golden[height]["sha256"]:
                    raise ValueError(f"Official log comparison mismatch at {height}")
                verified += 1
            blocks.append(dict(number=height, rawBlock=raw, receipts=receipts, logs=logs, logsSha256=log_hash))
        if offset % 100 == 0:
            print(f"Captured {len(blocks)}/{len(heights)} blocks", flush=True)
        # One bounded six-request batch at a time, without a load-test loop.
        time.sleep(0.1)

    report = dict(
        schema=1,
        capturedAtUTC=datetime.datetime.now(datetime.timezone.utc).isoformat(),
        planSha256=hashlib.sha256(args.plan.read_bytes()).hexdigest(),
        officialLogComparisons=verified,
        blocks=blocks,
    )
    args.output.write_text(json.dumps(report, separators=(",", ":")))
    print(f"Captured {len(blocks)} blocks; {verified} official log comparisons matched", flush=True)


if __name__ == "__main__":
    main()
