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

## Deployment

The `SSH Key - goreleaser` in 1Password was created and added (per [documentation](https://circleci.com/docs/github-integration/#create-additional-github-ssh-keys)) as a Github deploy key with write access and a CircleCI deploy key.  The CircleCI `goreleaser` context contains a classic GITHUB_TOKEN with `delete:packages, repo, write:packages` permissions
for publishing to the `metronome-industries/homebrew-metronome` tap.

## Links

* <https://docs.substrate.tools/substrate/access/aws-cli-profiles>
* <https://github.com/spf13/cobra/>
* <https://github.com/bitfield/script>
* <https://github.com/aws/aws-sdk-go-v2>
* <https://goreleaser.com/>
