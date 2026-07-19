# Deploy the Fly relay box

The relay box is single-tenant: one Fly app serves one computer. Fly forwards
raw TCP, while the box opens only its separate Mac-side TLS layer. The sealed
phone-to-computer TLS stream passes through unchanged.

## Cost and account check

This creates paid Fly resources. Before running these commands, confirm the
signed-in account and organization that will be charged:

```bash
fly auth whoami
fly orgs list
```

The V1 shape is one shared CPU with 256 MB, one dedicated IPv4, and one encrypted
1 GB volume. Check Fly's current prices and get the account owner's approval
before creating them. The checked-in `[[vm]]` block fixes the machine at one
shared CPU and 256 MB. Keep high availability and automatic machine creation off.

## Create and deploy

Install `flyctl`, clone this repository, and choose a globally unique app name
and a region near the computer. Replace `app` and `primary_region` in `fly.toml`,
then run:

```bash
fly apps create YOUR_RELAY_APP --org YOUR_ORG
fly volumes create relaybox_data --app YOUR_RELAY_APP --region YOUR_REGION --size 1
relay_registration_secret="$(openssl rand -base64 32)"
fly secrets set --app YOUR_RELAY_APP RELAYBOX_SECRET="$relay_registration_secret"
unset relay_registration_secret
fly ips allocate-v4 --app YOUR_RELAY_APP
fly deploy --app YOUR_RELAY_APP --ha=false
fly config validate --config fly.toml
fly services list --app YOUR_RELAY_APP
fly status --app YOUR_RELAY_APP
```

The literal secret is not placed in shell history by this sequence. It is still
passed to `flyctl` for the secret update; do this only on a computer you control.
Do not put it in `fly.toml`, source control, chat, logs, or an issue.

The service list must show exactly these public routes:

```text
TCP  8443 => 8443  [PROXY_PROTO]
TCP   443 => 9000  []
```

Do not add Fly `tls`, `http`, or `http_service` handlers to either route. The
PROXY handler on the phone route adds only the caller's address for rate limits;
it does not decrypt TLS.

## Save the pin and configure the computer

At first start, the box creates `/data/relaybox/box.pem` on the encrypted volume
and logs one `PINNED KEY` value. Read it from the initial deployment output or:

```bash
fly logs --app YOUR_RELAY_APP --no-tail
```

Copy only that base64 pin into the companion setup command. Never copy the PEM
file or registration secret into the repository. Continue with
[computer companion setup](companion.md), using public Mac port 443 and phone
port 8443.

Deleting the volume rotates the box identity and deliberately invalidates the
saved pin. Back up the pin as configuration; do not back up the private key
outside the encrypted volume.

## Move an existing phone and rotate the secret

An already-paired phone retains its old network address. After switching the
computer to this relay, revoke that device, create a fresh five-minute pairing
link, and pair it again. Codex tasks on the computer are not deleted.

To rotate a suspected relay secret, first make a new strong value, update the Fly
secret, and immediately rerun the full companion `setup` command with that same
value. The relay restarts and the computer is briefly offline during this
change. Run `./codex-launcher doctor` afterward; its relay check verifies the
pin and secret without taking the live control slot.

## Recover and remove

- If the public route fails while the machine is healthy, allocate a replacement
  dedicated IPv4, verify it, and release the bad address only after the new route
  works.
- If the volume is lost, deploy with a new volume, collect the new pin, rerun
  companion setup, and re-pair the phone.
- To stop charges permanently, remove the app and confirm its machine, IP, and
  volume are gone in Fly. This is destructive; export any evidence you need and
  get the account owner's approval first.
