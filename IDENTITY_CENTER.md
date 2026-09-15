# IAM Identity Center

`quikstrate` now uses IAM Identity Center (IDC) by default. During the AWS organization migration
it supports both the temporary Metronome instance and Stripe's instance. Stripe IDC is attempted
first except for account IDs in the local `metronome_idc_account_ids` list. The
`stripe_idc_account_ids` list records explicit cutovers; an account in neither list defaults to
Stripe. If the preferred instance is unavailable or lacks an assignment, quikstrate attempts the
other IDC instance. It never silently falls back to Substrate.

## Prerequisites

- The [access-metronome-aws-admin](https://go/ldapg/access-metronome-aws-admin) LMS permission
- AWS CLI v2.9.0 or later

## Update Quikstrate

Identity Center support requires Quikstrate v1.0.33 or later. Update the Homebrew tap and upgrade
Quikstrate before opting in:

```bash
brew update
brew upgrade quikstrate
brew info quikstrate
```

`brew info quikstrate` displays the installed version. If Homebrew reports that Quikstrate is not
installed, run:

```bash
brew tap metronome-industries/metronome
brew install quikstrate
```

## Setup

A browser window opens on the first credential request that requires an SSO login.

```bash
quikstrate configure
```

This:

- Writes both `[sso-session metronome]` and `[sso-session stripe]` blocks to `~/.aws/config`.
- On the first run, seeds `metronome_idc_account_ids` in `~/.quikstrate/config.json` with every
  known Metronome account and initializes `stripe_idc_account_ids` as empty.
- Re-runs the normal `configure` flow, so existing `aws` and `kubectl` commands continue to use
  the same profiles and contexts. The configured credential processes resolve through Identity
  Center under the hood.

The first credential request after setup opens a browser for `aws sso login`. This may be triggered
by `quikstrate credentials` or by an `aws` or `kubectl` command that invokes Quikstrate. The AWS CLI
caches SSO tokens in `~/.aws/sso/cache/` and reuses them until they expire, so browser login is not
required for every command.

## Move accounts between IDC instances

After an account is migrated and access is provisioned through Stripe's migration process, move it
from the Metronome list to the Stripe list. These commands are idempotent and preserve unrelated
AWS configuration:

```bash
quikstrate idc stripe staging
quikstrate idc stripe prod
quikstrate idc stripe admin
quikstrate idc stripe 407752757973
```

To undo a rehearsal or temporarily restore Metronome-first routing, use the inverse command:

```bash
quikstrate idc metronome staging
```

`QUIKSTRATE_IDC_INSTANCE=metronome|stripe` forces the preferred instance for one command and is
intended for migration diagnosis.

## Stripe regional failover

During an outage of Stripe IDC's primary `us-west-2` region, follow the
[IDC Regional Outage Runbook](https://trailhead.corp.stripe.com/docs/cloud-security-internal/run-and-on-call-runbooks/operational-runbooks/idc-regional-outage-runbook)
and enable the common CLI failover switch:

```bash
export SC_USE_ALTERNATE_REGION=true
```

Stripe IDC requests then use the replicated `us-east-2` instance at
`https://ssoins-7907aa69624c0735.portal.us-east-2.app.aws`. Its SSO token and role credentials use
the distinct `stripe-us-east-2` session/cache name. Metronome IDC remains in `us-west-2` and has no
regional failover. Unset the variable after the incident:

```bash
unset SC_USE_ALTERNATE_REGION
```

## Explicit Substrate fallback

To use Substrate for all future commands:

```bash
quikstrate configure --use-substrate
```

This changes `~/.quikstrate/config.json` to `credential_source: substrate`. The IDC session blocks
remain harmlessly in `~/.aws/config`. Substrate cannot access accounts after they migrate, so it is
not a workaround for a missing Stripe assignment.

For a single command without changing the persisted configuration:

```bash
USE_SUBSTRATE=true quikstrate credentials
```

`USE_SUBSTRATE=true` takes precedence over `config.json`.

To remove and rebuild local configuration, add `--clean`. This deletes and rebuilds the entire
`~/.aws/config` and `~/.kube/config`, not only the Identity Center block:

```bash
quikstrate configure --use-substrate --clean
```

## Roll back Quikstrate to v1.0.28

If the Identity Center release causes problems beyond the credential flow, v1.0.28 is the last
known-good Quikstrate release from before the AWS account and Identity Center changes.

First, unlink the Homebrew-managed binary and ensure `~/.local/bin` exists on your `PATH`:

```bash
brew unlink quikstrate
mkdir -p ~/.local/bin
export PATH="$HOME/.local/bin:$PATH"
```

Then download the v1.0.28 release for your Mac's architecture:

```bash
case "$(uname -m)" in
  arm64) artifact="quikstrate_Darwin_arm64.tar.gz" ;;
  x86_64) artifact="quikstrate_Darwin_x86_64.tar.gz" ;;
  *) echo "Unsupported architecture: $(uname -m)"; return 1 ;;
esac

curl -fL \
  "https://github.com/Metronome-Industries/quikstrate/releases/download/1.0.28/$artifact" \
  -o "/tmp/$artifact"
tar -xzf "/tmp/$artifact" -C ~/.local/bin quikstrate
hash -r
```

Add `export PATH="$HOME/.local/bin:$PATH"` to `~/.zshrc` if it is not already present. Confirm that
the rollback binary takes precedence over Homebrew:

```bash
which -a quikstrate
```

To return to the current Homebrew release later, delete the rollback binary and relink Homebrew:

```bash
rm ~/.local/bin/quikstrate
brew update
brew upgrade quikstrate
brew link quikstrate
hash -r
```

## Credential behavior

- **Command usage does not change.** Continue to use `quikstrate assume`, `quikstrate credentials`,
  `aws --profile ...`, and existing Kubernetes contexts regardless of the credential source.
- **Credential caches are instance-specific.** IDC credentials use names such as
  `prod-api-gamma-admin-stripe-idc.json`, based on the instance that actually returned them. A
  fallback to Metronome is therefore written to a `*-metronome-idc.json` file. Authenticate again
  once per IDC instance after an organization move; do not rely on old generic `*-idc.json` files.
- **Role names.** `Administrator` maps to IDC's `admin` permission set. Read-only IDC callers must
  use `--role engineersreadonly`; `--role Auditor` returns an actionable error in IDC mode. Explicit
  Substrate mode continues to accept `Auditor`.
- **Missing permissions do not fall back to Substrate.** A combined error identifies both IDC
  instances when neither can provide the requested permission set.

## Troubleshooting

### Force a browser login

Quikstrate automatically runs `aws sso login` when the cached SSO token is missing or expired. To
log in manually, run:

```bash
aws sso login --sso-session stripe
```

### Use Substrate as a fallback

If an Identity Center workflow is blocked and the account has not migrated, prefix the command with
`USE_SUBSTRATE=true`:

```bash
eval "$(USE_SUBSTRATE=true quikstrate assume --env staging --domain api)"
```

### Requested permission set is unavailable

This means that the permission set is not provisioned for the requested AWS account. Verify the
Stripe migration assignment and use the other IDC instance for diagnosis. Quikstrate intentionally
does not fall back to Substrate in this case.

### AWS CLI version errors

IDC requires AWS CLI v2.9.0 or newer. `quikstrate configure` and interactive SSO login check this
prerequisite and report whether `aws` is missing, unparsable, or too old. Upgrade using the link in
the error; `USE_SUBSTRATE=true` is only a temporary fallback while Substrate remains usable.

## Migration context

The Metronome Identity Center instance is temporary. It supports validating Identity Center
workflows before Metronome AWS accounts migrate into the Stripe AWS Organization. After that
migration, Stripe's Identity Center will replace both the temporary instance and Substrate as the
normal credential source.

- [Metronome AWS Identity Center Plan](https://docs.google.com/document/d/1dnqNDbDVzoKWjflu4RfNnWgf8EFZRNOyUlrpsbiy5P4/edit?tab=t.0)
- [Metronome AWS Integration Plan](https://docs.google.com/document/d/19W-7RO6TT7h9zIzpwkoi3CV7CbpLUBZaOby-zLeoj2E/edit?tab=t.0)

If you have issues, reach out [@rylan](https://go/~/rylan)
