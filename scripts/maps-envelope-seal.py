#!/usr/bin/env python3
# Fact-force (edit):
# 1) Callers: dogfood OpenAI/maps key import; MapsImportSession.importSealedJson;
#    LiveMapsBrokerProofTest orchestration.
# 2) Was maps-only; plan adds --provider openai reading OPENAI_API_KEY.
# 3) Schemas: offer JSON → envelope JSON (ciphertext only). INFO stays
#    operator-maps-key-import-v1 (shared envelope crypto; provider is in AAD).
# 4) User: "Implement OpenAI+Beeper phone-runtime plan — SLICE 1 only"
"""Seal an API key from a local .env into an adb-safe envelope.

Never prints the API key. Writes only ciphertext JSON.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import sys
from pathlib import Path

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF


INFO = b"operator-maps-key-import-v1"

PROVIDER_ENV = {
    "maps": "GOOGLE_MAPS_API_KEY",
    "openai": "OPENAI_API_KEY",
}


def b64u_encode(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode("ascii").rstrip("=")


def b64u_decode(text: str) -> bytes:
    pad = "=" * ((4 - len(text) % 4) % 4)
    return base64.urlsafe_b64decode(text + pad)


def read_env_key(env_path: Path, env_name: str) -> bytes:
    for line in env_path.read_text().splitlines():
        if line.startswith(env_name + "="):
            val = line.split("=", 1)[1].strip().strip('"').strip("'")
            if not val:
                raise SystemExit(f"{env_name} empty")
            return val.encode("utf-8")
    raise SystemExit(f"{env_name} missing")


def aad_utf8(meta: dict) -> bytes:
    lines = [
        "v=1",
        f"importId={meta['importId']}",
        f"provider={meta['provider']}",
        f"expiresAtUnix={meta['expiresAtUnix']}",
        f"deviceSerial={meta['deviceSerial']}",
        f"packageName={meta['packageName']}",
        f"signingDigestSha256={meta['signingDigestSha256']}",
    ]
    return "\n".join(lines).encode("utf-8")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--offer", required=True, help="path to import-offer.json")
    ap.add_argument("--env", required=True, help="path to Mac .env with the provider key")
    ap.add_argument("--out", required=True, help="path for import-envelope.json")
    ap.add_argument(
        "--provider",
        choices=sorted(PROVIDER_ENV),
        default=None,
        help="maps or openai (default: offer.provider or maps)",
    )
    args = ap.parse_args()

    offer = json.loads(Path(args.offer).read_text())
    provider = args.provider or offer.get("provider") or "maps"
    if provider not in PROVIDER_ENV:
        raise SystemExit(f"unsupported provider={provider!r}")
    env_name = PROVIDER_ENV[provider]
    api_key = read_env_key(Path(args.env), env_name)
    device_pub = serialization.load_der_public_key(b64u_decode(offer["devicePublicKeySpkiB64"]))
    helper = ec.generate_private_key(ec.SECP256R1())
    shared = helper.exchange(ec.ECDH(), device_pub)
    aes_key = HKDF(
        algorithm=hashes.SHA256(),
        length=32,
        salt=offer["importId"].encode("utf-8"),
        info=INFO,
    ).derive(shared)
    meta = {
        "importId": offer["importId"],
        "provider": provider,
        "expiresAtUnix": offer["expiresAtUnix"],
        "deviceSerial": offer["deviceSerial"],
        "packageName": offer["packageName"],
        "signingDigestSha256": offer["signingDigestSha256"],
    }
    nonce = os.urandom(12)
    ct = AESGCM(aes_key).encrypt(nonce, api_key, aad_utf8(meta))
    helper_pub = helper.public_key().public_bytes(
        serialization.Encoding.DER,
        serialization.PublicFormat.SubjectPublicKeyInfo,
    )
    out = {
        **meta,
        "v": 1,
        "helperEphemeralPublicKeyB64": b64u_encode(helper_pub),
        "nonceB64": b64u_encode(nonce),
        "ciphertextB64": b64u_encode(ct),
    }
    Path(args.out).write_text(json.dumps(out, indent=2) + "\n")
    del api_key, shared, aes_key
    print(
        "sealed",
        f"provider={provider}",
        f"importId={meta['importId']}",
        f"out={args.out}",
        f"ciphertext_len={len(ct)}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
