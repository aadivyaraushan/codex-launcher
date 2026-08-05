#!/usr/bin/env python3
# Gate: importers=MapsImportSession Android open; callers=Maps live orchestration;
# API=ECDH P-256 + HKDF-SHA256 + AES-256-GCM seal; schemas=offer JSON -> envelope
# JSON (ciphertext only over adb); user: "Import key from Mac .env onto Android
# via the allowed envelope path only (not Linux plaintext)."
"""Seal GOOGLE_MAPS_API_KEY from a local .env into an adb-safe envelope.

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


def b64u_encode(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode("ascii").rstrip("=")


def b64u_decode(text: str) -> bytes:
    pad = "=" * ((4 - len(text) % 4) % 4)
    return base64.urlsafe_b64decode(text + pad)


def read_env_key(env_path: Path) -> bytes:
    for line in env_path.read_text().splitlines():
        if line.startswith("GOOGLE_MAPS_API_KEY="):
            val = line.split("=", 1)[1].strip().strip('"').strip("'")
            if not val:
                raise SystemExit("GOOGLE_MAPS_API_KEY empty")
            return val.encode("utf-8")
    raise SystemExit("GOOGLE_MAPS_API_KEY missing")


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
    ap.add_argument("--offer", required=True, help="path to maps-import-offer.json")
    ap.add_argument("--env", required=True, help="path to Mac .env with GOOGLE_MAPS_API_KEY")
    ap.add_argument("--out", required=True, help="path for maps-import-envelope.json")
    args = ap.parse_args()

    offer = json.loads(Path(args.offer).read_text())
    api_key = read_env_key(Path(args.env))
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
        "provider": "maps",
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
        f"importId={meta['importId']}",
        f"out={args.out}",
        f"ciphertext_len={len(ct)}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
