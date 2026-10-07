# quikstrate

Formerly a wrapper of `substrate`, now just a CLI for caching short-lived AWS credentials for fast authentication and configuring `aws` and `kubectl` config files for easier profile and context switching.

Under the hood `quikstrate` is caching and reusing the credentials returned by AWS IAM Identity Center in `~/.quikstrate/`, with the option to fallback to Substrate for credential vending.

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

# fetch and cache default AWS credentials
quikstrate credentials

# updates ~/.aws/config and ~/.kube/config
quikstrate configure
```

To see what version of quikstrate you are running, run: `brew info quikstrate`

## AWS credentials

By default, `quikstrate` fetches short-lived AWS credentials through IAM Identity Center (IDC). Set up the required AWS profiles, Kubernetes contexts, and SSO session configuration with:

```bash
quikstrate configure
```

`configure` only replaces the AWS config values that quikstrate owns. Managed
values are delimited by `BEGIN/END QUIKSTRATE MANAGED VALUES` comments; unrelated
profiles, values, and comments in `~/.aws/config` are preserved, including when
`--clean` is used.

See [IDENTITY_CENTER.md](IDENTITY_CENTER.md) for detailed setup, credential behavior, and troubleshooting during the migration from Substrate to Identity Center.

## Releasing

After changes merge to `main`, CircleCI creates and pushes the next version tag and GoReleaser
publishes the GitHub release, platform archives, and checksums.

The Homebrew formula is not updated automatically because the tap's `main` branch requires pull
requests. Follow the
[`homebrew-metronome` quikstrate upgrade guide](https://github.com/Metronome-Industries/homebrew-metronome/blob/main/upgrading-quikstrate.md)
to update the formula version, archive URLs, and SHA-256 checksums for all four platform archives;
test the formula through a local tap checkout; and open a PR against `homebrew-metronome`.

## Links

- <https://github.com/substrate-maintainers/substrate/blob/main/docs/access/aws-cli-profiles.md>
- <https://github.com/spf13/cobra/>
- <https://github.com/bitfield/script>
- <https://github.com/aws/aws-sdk-go-v2>
- <https://goreleaser.com/>
