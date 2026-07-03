# aikido-rules

Pins and provisions a **complete, unmodified mirror** of the
[opengrep/opengrep-rules](https://github.com/opengrep/opengrep-rules) ruleset —
the community Opengrep rules maintained by Aikido Security (a fork of Semgrep
Rules). `aikido-guard` points `opengrep --config` at the populated `rules/` tree.

## Why `rules/` is not committed

The upstream ruleset is ~4000 files and, critically, ships **secret-detection
test fixtures** containing sample API keys (Mailchimp, Twilio, Square, Stripe, …).
GitHub push protection flags those as live secrets and **rejects the push**. So
`rules/` is **gitignored** (see the repo `.gitignore`) and provisioned locally
instead. Only the *pin* (`PINNED_COMMIT`) and the license/README travel in git.

## Contents (tracked)

- `PINNED_COMMIT` — the exact upstream commit the mirror is provisioned from.
- `UPSTREAM_README.md` — the upstream project's own README, verbatim.
- `LICENSE` — the upstream license, verbatim.
- `rules/` — **not tracked**; populated locally (see below) with the entire
  upstream tree (one dir per language/ecosystem, 2000+ `*.yaml`/`*.yml` rules).

## Provisioning `rules/`

The offline installer copies the ruleset out of the staged bundle
(`vendor/aikido-bundle/rules/`) into `rules/`:

```sh
scripts/aikido-install.sh          # installs scanners + DBs + this ruleset
scripts/aikido-install.sh --check  # preflight: reports whether it is staged
```

To stage the bundle from upstream (on a host with connectivity), clone at the
pinned commit and drop it into the bundle:

```sh
git clone https://github.com/opengrep/opengrep-rules.git /tmp/opengrep-rules
git -C /tmp/opengrep-rules checkout "$(cat plugins/aikido-rules/PINNED_COMMIT)"
rsync -a --exclude='.git' /tmp/opengrep-rules/ vendor/aikido-bundle/rules/
# repin / refresh license + upstream readme when bumping the pin:
git -C /tmp/opengrep-rules rev-parse HEAD > plugins/aikido-rules/PINNED_COMMIT
cp /tmp/opengrep-rules/LICENSE        plugins/aikido-rules/LICENSE
cp /tmp/opengrep-rules/README.md      plugins/aikido-rules/UPSTREAM_README.md
```

## License

These rules are distributed under their upstream license — see `LICENSE`. They
are intended for research, testing and benchmarking.
