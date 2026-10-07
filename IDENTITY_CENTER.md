# IAM Identity Center

Starting in v1.0.36, `quikstrate` fetches credentials via AWS IAM Identity Center (IDC) by default.

All commands, AWS profiles, and Kubernetes contexts are expected to continue to work; quikstrate resolves the required IDC account and permission set behind the scenes.

## Install or update Quikstrate

```bash
brew update
brew upgrade quikstrate
brew info quikstrate
```

If Quikstrate is not installed:

```bash
brew tap metronome-industries/metronome
brew install quikstrate
```

## Setup

AWS CLI v2.9.0 or newer is required. Run:

```bash
quikstrate configure
```

This configures the Metronome and primary Stripe SSO sessions, AWS profiles, and Kubernetes contexts
used by quikstrate. The first credential request for an IDC session may open a browser for
`aws sso login`. AWS CLI stores each session's SSO token in `~/.aws/sso/cache/` and reuses it until
it expires.

On a Stripe devbox, preserve the devbox's native AWS authentication while configuring the named
profiles and Kubernetes contexts:

```bash
quikstrate configure --preserve-aws-auth
```

The equivalent environment override is useful for automated devbox setup:

```bash
QUIKSTRATE_PRESERVE_AWS_AUTH=true quikstrate configure
```

This removes SSO sessions and `credential_process` values installed by earlier quikstrate runs,
but preserves unrelated AWS configuration. The profiles continue through the devbox's native AWS
credential provider chain.

## Credential behavior

- Continue to use `quikstrate assume`, `quikstrate credentials`, `aws --profile ...`, and existing Kubernetes contexts.
- `Administrator` maps to IDC's `admin` permission set.
- Read-only IDC callers must use `--role engineersreadonly`; `Auditor` is a Substrate role name.
- Credential cache filenames identify the IDC endpoint that issued them: `*-metronome-idc.json`,
  `*-stripe-idc.json`, or `*-stripe-us-east-2-idc.json`.
- If no configured IDC instance can provide a requested permission set, the error identifies each instance attempted. Quikstrate does not fall back to Substrate automatically.

## Supported IDC instances

During the migration of Metronome AWS accounts into the Stripe AWS Organization, `quikstrate` supports Identity Center in both AWS organizations plus Stripe's alternate region.

| Session | Region | Start URL | Purpose |
| --- | --- | --- | --- |
| `metronome` | `us-west-2` | `https://d-9267463e84.awsapps.com/start` | Accounts still in the Metronome AWS organization |
| `stripe` | `us-west-2` | `https://d-9267fda1d4.awsapps.com/start` | Primary endpoint for accounts in the Stripe AWS organization |
| `stripe-us-east-2` | `us-east-2` | `https://ssoins-7907aa69624c0735.portal.us-east-2.app.aws` | Stripe's replicated endpoint for a regional outage |

## Stripe regional failover

During an outage of Stripe IDC's primary `us-west-2` region, follow the
[IDC Regional Outage Runbook](https://trailhead.corp.stripe.com/docs/cloud-security-internal/run-and-on-call-runbooks/operational-runbooks/idc-regional-outage-runbook)
and enable the common CLI failover switch:

```bash
export SC_USE_ALTERNATE_REGION=true
```

Stripe IDC requests then use the replicated `stripe-us-east-2` endpoint instead of the primary
`stripe` endpoint. Metronome IDC requests remain in `us-west-2`. The alternate SSO token and role
credentials use a distinct session/cache name. Unset the variable after the incident:

```bash
unset SC_USE_ALTERNATE_REGION
```

## Troubleshooting

### Force a browser login

Quikstrate automatically opens a browser when an SSO token is missing or expired. To log in manually, choose the session you need:

```bash
aws sso login --sso-session metronome
aws sso login --sso-session stripe
```

### Requested permission set is unavailable

Verify that you have the required LMS permission and that it has propagated to IDC. A cached SSO
token may need to be refreshed after new access is granted.

### AWS CLI version errors

Upgrade to AWS CLI v2.9.0 or newer using the link included in quikstrate's error message.

## Temporary: Metronome-to-Stripe AWS migration

> Remove this section after all Metronome AWS accounts have migrated and the temporary Metronome
> IDC and Substrate compatibility paths are removed.

During the migration, quikstrate supports both the temporary Metronome IDC instance and Stripe IDC. The required current LMS permission is [access-metronome-aws-admin](https://go/ldapg/access-metronome-aws-admin).

### Migration preference

Before migration, a missing `preferred_idc_instance` setting defaults to Metronome IDC. `quikstrate configure` preserves an existing preference but does not write one automatically.

On migration day, switch the global preference once:

```bash
quikstrate idc stripe
```

To reverse a rehearsal or restore Metronome-first routing:

```bash
quikstrate idc metronome
```

These commands update only `preferred_idc_instance` in `~/.quikstrate/config.json`; they do not reconfigure AWS or Kubernetes or change the credential source. Quikstrate tries the preferred organization first, then reports and tries the other organization if the account or permission set is unavailable. It never falls back to Substrate automatically.

For diagnosis, `QUIKSTRATE_IDC_INSTANCE=metronome|stripe` overrides the saved preference for one command.

### Explicit Substrate fallback

Substrate remains available only as an explicit temporary fallback for accounts that have not
migrated. For one command:

```bash
USE_SUBSTRATE=true quikstrate credentials
```

To persist the selection:

```bash
quikstrate configure --use-substrate
```

To rebuild quikstrate-owned AWS configuration and Kubernetes configuration in Substrate mode
without deleting unrelated AWS profiles or values:

```bash
quikstrate configure --use-substrate --clean
```

### Release rollback

If a release causes problems beyond credential routing, use the last known-good release identified
by the migration owner rather than treating Substrate as an automatic fallback.

### Migration references

- [Metronome AWS Identity Center Plan](https://docs.google.com/document/d/1dnqNDbDVzoKWjflu4RfNnWgf8EFZRNOyUlrpsbiy5P4/edit?tab=t.0)
- [Metronome AWS Integration Plan](https://docs.google.com/document/d/19W-7RO6TT7h9zIzpwkoi3CV7CbpLUBZaOby-zLeoj2E/edit?tab=t.0)

For migration issues, reach out [@rylan](https://go/~/rylan).
