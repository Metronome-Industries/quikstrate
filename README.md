# quikstrate

`quikstrate` configures AWS and kubectl profiles and caches short-lived AWS credentials.

## Installing

```bash
brew tap metronome-industries/metronome
brew update
brew install quikstrate
```

## Usage

```bash
quikstrate credentials
quikstrate configure
```

IAM Identity Center is the default credential source. Run `quikstrate configure` once after upgrading; it validates AWS CLI v2.9.0 or newer, writes both the Stripe and temporary Metronome SSO sessions, and updates quikstrate-managed credential-process profiles without removing unrelated AWS configuration.

During migration, `~/.quikstrate/config.json` contains `metronome_idc_account_ids`. Accounts in that shrinking exception list try Metronome IDC first; all others try Stripe IDC first. If the preferred organization lacks an assignment, quikstrate tries the other IDC organization and reports both errors when neither works.

Migration operators update routing idempotently by environment or account:

```bash
quikstrate configure --mark-stripe-idc staging
quikstrate configure --mark-stripe-idc prod
quikstrate configure --mark-stripe-idc admin
quikstrate configure --mark-stripe-idc 407752757973
quikstrate configure --mark-metronome-idc 407752757973
```

`Administrator` remains the default and maps to the IDC `admin` permission set. Request read-only IDC access with `--role engineersreadonly`; `--role Auditor` is accepted only in explicit Substrate mode.

Temporary Substrate rollback remains explicit:

```bash
quikstrate configure --use-substrate
USE_SUBSTRATE=true quikstrate credentials
```

Substrate cannot access accounts after they migrate and does not replace a missing Stripe assignment. For migration diagnosis, a command can target one IDC instance with `QUIKSTRATE_IDC_INSTANCE=metronome|stripe`.

See [IDENTITY_CENTER.md](IDENTITY_CENTER.md) for migration details, prerequisites, cache behavior, and troubleshooting.

## Deployment

The `SSH Key - goreleaser` in 1Password is configured as a GitHub and CircleCI deploy key. The CircleCI `goreleaser` context publishes release artifacts; the Homebrew formula is updated in a separate follow-up PR.
