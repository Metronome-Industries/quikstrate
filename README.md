# quikstrate

Metronome wrapper of `substrate` CLI to cache credentials for faster authentication and configure `aws` and `kubectl` config files for easier profile and context switching.

## Installing

```bash
brew update
export HOMEBREW_GITHUB_API_TOKEN=$GITHUB_TOKEN
brew install metronome-industries/metronome/quikstrate

# view usage
quikstrate -h
```

## Usage

```bash
# same as `substrate credentials` but ~faster~
quikstrate credentials

# updates ~/.aws/config and ~/.kube/config
quikstrate configure
```

Under the hood `quikstrate` is caching and reusing the credentials returned by substrate in `~/.metronome/quikstrate/`

## Deployment

The `SSH Key - goreleaser` in 1Password was created and added (per [documentation](https://circleci.com/docs/github-integration/#create-additional-github-ssh-keys)) as a Github deploy key with write access and a CircleCI deploy key.  The CircleCI `goreleaser` context contains a classic GITHUB_TOKEN with `delete:packages, repo, write:packages` permissions
for publishing to the `metronome-industries/homebrew-metronome` tap.

## Links

* <https://docs.substrate.tools/substrate/access/aws-cli-profiles>
* <https://github.com/spf13/cobra/>
* <https://github.com/bitfield/script>
* <https://github.com/aws/aws-sdk-go-v2>
* <https://goreleaser.com/>

## TODOs

* add whoami
  * get the human-friendly account name from cached accounts file
* configure kubeconfig to ignore AWS_* environment variables
