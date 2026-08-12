# Contributing

Thanks for helping with Operator / OpenClaw phone agent.

## Before you open a PR

1. Prefer a feature branch off the integration line you were given (often
   `worktree-phase2-tool-bridge` during the phone-agent pivot).
2. Keep secrets out of commits: token **paths** and cert **paths** only; never
   values. Do not log message text or people’s names (ids only).
3. Match the existing layout and tone — chat-first UI, least code that solves
   the problem.

## Checks that should stay green

From the repo root:

```bash
go test -count=1 -p 1 ./companion/...
bash scripts/phone-boot/test/run-tests.sh
```

If you touch Android:

```bash
cd android
./gradlew :app:testDebugUnitTest
```

If you touch the OpenClaw plugin:

```bash
cd agentbridge/openclaw-plugin
npm test
```

## Docs

- Product entry: [README.md](README.md)
- On-phone setup: [docs/setup/phone-agent.md](docs/setup/phone-agent.md)
- Boot package: [scripts/phone-boot/README.md](scripts/phone-boot/README.md)
