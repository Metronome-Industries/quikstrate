# IAM Identity Center

By default, `quikstrate` fetches credentials via Substrate. After Metronome migrates into the
Stripe AWS Organization, Substrate will be replaced by Stripe's IAM Identity Center. As of
quikstrate v1.0.33, a temporary IAM Identity Center instance in Metronome AWS is available as an
opt-in `quikstrate` credential source.

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

A browser window will open when configuring kubectl that requires you to SSO with your Stripe Identity via Google. 

```bash
quikstrate configure --use-identitycenter
```

This:

- Writes `credential_source: identitycenter` to `~/.quikstrate/config.json`.
- Writes an `[sso-session metronome]` block to `~/.aws/config` pointing at Metronome's Identity
  Center instance.
- Re-runs the normal `configure` flow, so existing `aws` and `kubectl` commands continue to use
  the same profiles and contexts. The configured credential processes resolve through Identity
  Center under the hood.

The first credential request after setup opens a browser for `aws sso login`. This may be triggered
by `quikstrate credentials` or by an `aws` or `kubectl` command that invokes Quikstrate. The AWS CLI
caches SSO tokens in `~/.aws/sso/cache/` and reuses them until they expire, so browser login is not
required for every command.

## Roll back to Substrate

To use Substrate for all future commands:

```bash
quikstrate configure --use-substrate
```

This changes `~/.quikstrate/config.json` to `credential_source: substrate`. The Identity Center
block remains harmlessly in `~/.aws/config`.

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
- **Credential caches are separate.** Identity Center credentials use separate files such as
  `*-idc.json` in `~/.quikstrate/`. Switching sources cannot reuse credentials cached by the other
  source.
- **Role names are mapped automatically.** `Administrator` maps to `admin`, while `Auditor` maps to
  `engineersreadonly`. Scripts using the Substrate role names continue to work with either source.
- **Missing permissions do not fall back silently.** If an account lacks the requested Identity
  Center permission set, Quikstrate returns an error such as
  `no "admin" permission set for account <account>`.

## Troubleshooting

### Force a browser login

Quikstrate automatically runs `aws sso login` when the cached SSO token is missing or expired. To
log in manually, run:

```bash
aws sso login --sso-session metronome
```

### Use Substrate as a fallback

If an Identity Center workflow is blocked, prefix the command with `USE_SUBSTRATE=true`:

```bash
eval "$(USE_SUBSTRATE=true quikstrate assume --env staging --domain api)"
```

### Requested permission set is unavailable

An error such as `no "admin" permission set for account <account>` means that permission set is
not provisioned for the requested AWS account. Quikstrate intentionally does not fall back to
Substrate in this case. Use the explicit Substrate fallback if needed.

## Migration context

The Metronome Identity Center instance is temporary. It supports validating Identity Center
workflows before Metronome AWS accounts migrate into the Stripe AWS Organization. After that
migration, Stripe's Identity Center will replace both the temporary instance and Substrate as the
normal credential source.

- [Metronome AWS Identity Center Plan](https://docs.google.com/document/d/1dnqNDbDVzoKWjflu4RfNnWgf8EFZRNOyUlrpsbiy5P4/edit?tab=t.0)
- [Metronome AWS Integration Plan](https://docs.google.com/document/d/19W-7RO6TT7h9zIzpwkoi3CV7CbpLUBZaOby-zLeoj2E/edit?tab=t.0)

If you have issues, reach out [@rylan](https://go/~/rylan)
