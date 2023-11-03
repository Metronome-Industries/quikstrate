# creds

## TODO on Fedex Friday

* add force flag to overwrite creds
* make installable via brew
* add whoami
  * get the human-friendly account name from cached accounts file

## Installing

```bash
go install
```

## Configuring AWS CLI and K8S CLI

```bash
# to see the usage and dry run
creds configure --help
creds configure --dry-run

# to remove ~/.aws/config and ~/.kube/config first
creds configure --clean
```

## Links

* <https://docs.substrate.tools/substrate/access/aws-cli-profiles>
* <https://github.com/spf13/cobra/>
* <https://github.com/bitfield/script>
* <https://github.com/aws/aws-sdk-go-v2>
* <https://goreleaser.com/>
