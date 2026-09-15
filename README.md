# quikstrate

Wrapper of `substrate` CLI to cache credentials for faster authentication and configure `aws` and `kubectl` config files for easier profile and context switching.

Under the hood `quikstrate` is caching and reusing the credentials returned by substrate in `~/.quikstrate/`

## Installing

```bash
# probably already done
brew tap metronome-industries/metronome

brew update
brew install quikstrate
```

## Usage

Run the command with `-h` or `--help` for detailed usage statements!

```bash
# view usage
quikstrate -h

# same as `substrate credentials` but ~quicker~ (run it twice to see the difference)
quikstrate credentials

# updates ~/.aws/config and ~/.kube/config
quikstrate configure
```

To see what version of quikstrate you are running, run: `brew info quikstrate`

## Credential sources: Substrate vs Identity Center

By default, `quikstrate` fetches credentials through IAM Identity Center (IDC). During the AWS
organization migration it uses Stripe IDC by default and keeps a local list of accounts that remain
on Metronome IDC. The config also records accounts explicitly moved to Stripe; accounts
in neither list default to Stripe. Configure both SSO sessions and seed the routing lists with:

```bash
quikstrate configure
```

When an account migrates, remove it from the local Metronome exception list without upgrading the
binary again. `staging`, `prod`, `admin`, and individual account IDs are accepted:

```bash
quikstrate idc stripe staging
quikstrate idc stripe 407752757973
```

Use `quikstrate idc metronome <selection>` to reverse a migration rehearsal.
Substrate remains an explicit temporary fallback: `quikstrate configure --use-substrate` persists
it, while `USE_SUBSTRATE=true quikstrate credentials` changes a single command.

During a Stripe IDC regional outage, set `SC_USE_ALTERNATE_REGION=true` to use Stripe's replicated
`us-east-2` instance. This switch does not affect Metronome IDC.

See [IDENTITY_CENTER.md](IDENTITY_CENTER.md) for prerequisites, credential behavior, rollback, and
troubleshooting.

## Deployment

The `SSH Key - goreleaser` in 1Password was created and added (per [documentation](https://circleci.com/docs/github-integration/#create-additional-github-ssh-keys)) as a Github deploy key with write access and a CircleCI deploy key. The CircleCI `goreleaser` context contains a classic GITHUB_TOKEN with `delete:packages, repo, write:packages` permissions
for publishing to the `metronome-industries/homebrew-metronome` tap.

## Links

- <https://github.com/substrate-maintainers/substrate/blob/main/docs/access/aws-cli-profiles.md>
- <https://github.com/spf13/cobra/>
- <https://github.com/bitfield/script>
- <https://github.com/aws/aws-sdk-go-v2>
- <https://goreleaser.com/>
