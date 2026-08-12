# AVD software attestation hatch (local-pair only)

Date: 2026-08-12
For: unblocking Phase 3–7 on-device UI proofs on a Pixel-like AVD, where Android Key Attestation cannot produce a Google-rooted chain.

## What this is

Emulator Keystore uses software keys. Local-pair then fails with:

`http_403:attestation: certificate chain invalid: x509: certificate signed by unknown authority`

(`ErrAttestChain` in `companion/internal/phoneruntime/localtrust`). Even if the chain parsed, `VerifyAttestation` would reject `SecuritySoftware` with `ErrAttestWeakLevel`.

The hatch is **AVD-only**. Production Pixel policy stays fail-closed: no software level, Google hardware attestation roots required.

## How to enable (AVD)

In the environment that starts `scripts/phone-boot/services/phone-runtime/run`:

```sh
export OPERATOR_ALLOW_SOFTWARE_ATTEST=1
```

Or pass `-allow-software-attest` to `operator-phone-runtime`. Either one is enough. The run script forwards the env to the CLI flag; the binary also reads the env itself.

What it changes, for local-pair only:

1. Software Keystore security level is accepted.
2. If the leaf cannot chain to the embedded Google attestation roots, the last certificate in the presented chain is used as the trust anchor and the chain must still verify to it.

Package name, signing digest, and pairing challenge checks still run.

## How to keep Pixel closed

Leave `OPERATOR_ALLOW_SOFTWARE_ATTEST` unset. Do not pass `-allow-software-attest`. Do not put that flag on a release Pixel runit line.

## Reproduce the default (closed) vs hatch

From the repo root:

```sh
go test -count=1 ./companion/internal/phoneruntime/localtrust/ \
  -run 'TestDefaultRejectsSoftwareLevel|TestAllowSoftwareAcceptsSoftwareLevel|TestEmulatorChain'
go test -count=1 ./companion/internal/phoneruntime/ \
  -run 'TestReleasePendingViaAttestation'
```

Default tests must reject software + emulator chains. Hatch tests must accept software level and an emulator-like chain for x509, while still failing a leaf that has no attestation extension.

## Do not

- Commit gateway token values, pairing secrets, or real device certs.
- Enable this on a phone that is supposed to prove hardware attestation.
