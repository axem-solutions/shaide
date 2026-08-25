#!/usr/bin/env python3
"""
Structural inference verification for embedding models in HF hub cache layout.
Uses Python standard library only — no pip install or network access needed.

Given a model directory name (e.g. models--nomic-ai--nomic-embed-text-v1.5),
it reads the model snapshot from /pull and verifies:
  1. config.json  — valid JSON, shows model architecture
  2. tokenizer    — tokenizer.json or vocab.txt present
  3. .safetensors — valid binary format, shows tensor count and key shapes

The safetensors format starts with an 8-byte little-endian uint64 (header
length), followed by a JSON header mapping tensor names → dtype/shape/offsets.

Usage: python3 infer.py <model_dir_name>
  e.g. python3 infer.py models--nomic-ai--nomic-embed-text-v1.5
"""
import json
import os
import struct
import sys

PULL_DIR = "/pull"


def find_snapshot(model_cache_dir: str) -> str:
    refs_main = os.path.join(model_cache_dir, "refs", "main")
    if os.path.exists(refs_main):
        with open(refs_main) as f:
            commit = f.read().strip()
        snapshot = os.path.join(model_cache_dir, "snapshots", commit)
        if os.path.isdir(snapshot):
            return snapshot
    # Fallback: first entry in snapshots/
    snaps_dir = os.path.join(model_cache_dir, "snapshots")
    for entry in sorted(os.listdir(snaps_dir)):
        full = os.path.join(snaps_dir, entry)
        if os.path.isdir(full):
            return full
    raise FileNotFoundError(f"No snapshot directory found under {model_cache_dir}")


def read_safetensors_header(filepath: str) -> dict:
    """Read and parse the JSON header from a safetensors file."""
    with open(filepath, "rb") as f:
        raw = f.read(8)
        if len(raw) < 8:
            raise ValueError(f"File too small to be valid safetensors: {filepath}")
        header_size = struct.unpack("<Q", raw)[0]
        header_bytes = f.read(header_size)
    return json.loads(header_bytes)


if len(sys.argv) < 2:
    print("ERROR: model directory name required as argument", file=sys.stderr)
    print("  e.g. python3 infer.py models--nomic-ai--nomic-embed-text-v1.5", file=sys.stderr)
    sys.exit(1)

model_dir_name  = sys.argv[1]
model_cache_dir = os.path.join(PULL_DIR, model_dir_name)

if not os.path.isdir(model_cache_dir):
    print(f"ERROR: model directory not found: {model_cache_dir}", file=sys.stderr)
    sys.exit(1)

snapshot_path = find_snapshot(model_cache_dir)

print(f"Model    : {model_dir_name}")
print(f"Snapshot : {snapshot_path}")
print()

errors = 0

# ── 1. config.json ────────────────────────────────────────────────────────────
print("── config.json")
config_path = os.path.join(snapshot_path, "config.json")
if os.path.exists(config_path):
    with open(config_path) as f:
        config = json.load(f)
    for key in ("model_type", "hidden_size", "num_hidden_layers",
                "num_attention_heads", "max_position_embeddings"):
        if key in config:
            print(f"   {key:<30}: {config[key]}")
    print("   status                        : OK")
else:
    print("   status                        : MISSING", file=sys.stderr)
    errors += 1

print()

# ── 2. Tokenizer ──────────────────────────────────────────────────────────────
print("── tokenizer")
tokenizer_files = ["tokenizer.json", "tokenizer_config.json", "vocab.txt"]
found_tokenizer = [f for f in tokenizer_files if os.path.exists(os.path.join(snapshot_path, f))]
if found_tokenizer:
    for tf in found_tokenizer:
        size_kb = os.path.getsize(os.path.join(snapshot_path, tf)) / 1024
        print(f"   {tf:<30}: {size_kb:.1f} KB")
    print(f"   status                        : OK")
else:
    print("   status                        : MISSING (no tokenizer files found)", file=sys.stderr)
    errors += 1

print()

# ── 3. Safetensors ────────────────────────────────────────────────────────────
print("── safetensors")
st_files = sorted(f for f in os.listdir(snapshot_path) if f.endswith(".safetensors"))

if not st_files:
    print("   status                        : MISSING (no .safetensors files)", file=sys.stderr)
    errors += 1
else:
    for st_file in st_files:
        st_path = os.path.join(snapshot_path, st_file)
        try:
            header  = read_safetensors_header(st_path)
            tensors = {k: v for k, v in header.items() if k != "__metadata__"}
            size_mb = os.path.getsize(st_path) / 1024 / 1024
            print(f"   file    : {st_file}  ({size_mb:.1f} MB,  {len(tensors)} tensors)")
            # Show up to 3 key tensors to confirm shapes make sense
            for k in list(tensors.keys())[:3]:
                shape = tensors[k].get("shape", "?")
                dtype = tensors[k].get("dtype", "?")
                print(f"   tensor  : {k:<40}  shape={shape}  dtype={dtype}")
        except Exception as e:
            print(f"   ERROR reading {st_file}: {e}", file=sys.stderr)
            errors += 1

print()

# ── Result ────────────────────────────────────────────────────────────────────
if errors == 0:
    print("VERIFICATION OK")
else:
    print(f"VERIFICATION FAILED ({errors} error(s))", file=sys.stderr)
    sys.exit(1)
