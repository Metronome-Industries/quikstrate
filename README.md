# metstrate

Metronome wrapper of `substrate` CLI to cache credentials and configure `aws` and `kubectl` config files.

## TODO on Fedex Friday

* add force flag to overwrite creds
* make installable via brew
* add whoami
  * get the human-friendly account name from cached accounts file

## Installing

```bash
export HOMEBREW_GITHUB_API_TOKEN=$GITHUB_TOKEN
brew install metstrate
```

## Configuring AWS CLI and K8S CLI

```bash
# to see the usage and dry run
metstrate configure --help
metstrate configure --dry-run

# to remove ~/.aws/config and ~/.kube/config first
metstrate configure --clean
```

## Deployment

The `SSH Key - goreleaser` in 1Password was created and added below (per [documentation](https://circleci.com/docs/github-integration/#create-additional-github-ssh-keys)):

* metstrate CircleCI deploy Keys
* metstrate GH deploy keys
* metronome-homebrew GH deploy keys

## Links

* <https://docs.substrate.tools/substrate/access/aws-cli-profiles>
* <https://github.com/spf13/cobra/>
* <https://github.com/bitfield/script>
* <https://github.com/aws/aws-sdk-go-v2>
* <https://goreleaser.com/>
