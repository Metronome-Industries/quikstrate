# IAM Identity Center

IAM Identity Center is quikstrate's default credential source. The migration-safe client supports both the temporary Metronome IDC organization and Stripe IDC while AWS accounts move between organizations. Substrate remains an explicit temporary fallback only.

## Prerequisites and setup

Install AWS CLI v2.9.0 or newer, upgrade quikstrate, and configure once:

```bash
brew update
brew upgrade quikstrate
quikstrate configure
```

Configuration preserves unrelated AWS configuration, writes `[sso-session metronome]` and `[sso-session stripe]`, and refreshes existing quikstrate credential-process profiles and kubectl contexts. `configure --use-identitycenter` remains an idempotent compatibility command.

Quikstrate checks `aws --version` during configuration and before an interactive login. Missing, old, and unparsable AWS CLI installations produce distinct errors. Upgrade instructions are at <https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html>.

The first request for each IDC instance may open a browser. SSO tokens remain in `~/.aws/sso/cache/`.

## Mixed-organization routing

On first configuration, quikstrate seeds `metronome_idc_account_ids` in `~/.quikstrate/config.json` with every known Metronome AWS account. An explicitly empty list is preserved. Routing is:

1. `QUIKSTRATE_IDC_INSTANCE=metronome|stripe`, when set, targets only that instance.
2. Accounts in `metronome_idc_account_ids` try Metronome and then Stripe.
3. Other accounts try Stripe and then Metronome.

The second attempt allows stale local routing configuration to continue working. Quikstrate never adds Substrate to this automatic fallback chain. If both IDC attempts fail, the error includes context from both organizations.

Operators move a whole wave or one account idempotently:

```bash
quikstrate configure --mark-stripe-idc staging
quikstrate configure --mark-stripe-idc prod
quikstrate configure --mark-stripe-idc admin
quikstrate configure --mark-stripe-idc 407752757973
```

The `admin` group includes management, audit, deploy, network, and the Substrate/admin account. To rehearse a rollback or correct local routing:

```bash
quikstrate configure --mark-metronome-idc staging
quikstrate configure --mark-metronome-idc 407752757973
```

Moving the list changes which IDC is attempted first; it does not provision groups, permission sets, or assignments. An account must not move until Stripe's migration process has made access available.

## Roles

`Administrator` remains the public default for staging, production, and special accounts and maps to the IDC permission set `admin`. Base `quikstrate credentials` also requests `admin` in account `666642175330`.

For read-only IDC access, use:

```bash
quikstrate assume --env prod --domain api --role engineersreadonly
```

`Auditor` is a Substrate role and is not aliased in IDC mode. Quikstrate rejects it before contacting AWS and directs callers to `--role engineersreadonly`. Explicit Substrate mode continues to accept `Auditor`.

Least-privilege defaults, reducing persistent admin, consumer inventory, and JIT admin are separate post-migration changes.

## Credential caches

IDC credential caches include the preferred instance, for example:

```text
~/.quikstrate/prod-api-gamma-admin-stripe-idc.json
~/.quikstrate/prod-api-gamma-admin-metronome-idc.json
~/.quikstrate/credentials-stripe-idc.json
```

Generic `*-idc.json` caches from older versions are not reused. Expect one authentication per IDC instance after upgrading or after clearing caches. Remove instance-specific files when diagnosing stale local credentials:

```bash
rm -f ~/.quikstrate/*-stripe-idc.json ~/.quikstrate/*-metronome-idc.json
```

## Explicit Substrate rollback

Persist the temporary fallback:

```bash
quikstrate configure --use-substrate
```

Or use it for one command:

```bash
USE_SUBSTRATE=true quikstrate credentials
```

`USE_SUBSTRATE=true` has precedence over persisted configuration. Substrate stops working for migrated accounts and is not the rollback for a missing Stripe assignment; use the alternate IDC attempt and the migration break-glass process.

To return to IDC:

```bash
quikstrate configure --use-identitycenter
```

## Troubleshooting

Force or diagnose a specific login:

```bash
aws sso login --sso-session stripe
aws sso login --sso-session metronome
QUIKSTRATE_IDC_INSTANCE=stripe quikstrate credentials --force
```

A missing-assignment error means the requested permission set is not available to the user in that account. Quikstrate reports failures from both IDC instances and does not silently hide the provisioning problem with Substrate.

If AWS CLI is unavailable during an emergency, `USE_SUBSTRATE=true` is a temporary fallback only while Substrate can still vend the requested account.

Migration references:

- [Metronome AWS Identity Center Plan](https://docs.google.com/document/d/1dnqNDbDVzoKWjflu4RfNnWgf8EFZRNOyUlrpsbiy5P4/edit?tab=t.0)
- [Metronome AWS Integration Plan](https://docs.google.com/document/d/19W-7RO6TT7h9zIzpwkoi3CV7CbpLUBZaOby-zLeoj2E/edit?tab=t.0)
