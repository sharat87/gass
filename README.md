# GitHub Actions Secrets Sync (`gass`) <sup>Beta</sup>

A tiny tool to set secrets for one or more GitHub repos, as specified in a YAML file. For example, consider a `secrets.yml` with:

```yaml
repos:
  sharat87/prestige:
    delete_unspecified: false  # If true, delete any secrets not mentioned in the below `secrets` list. Defaults to `false` if not specified.
    secrets:
      SOME_SECRET_NAME: super-secret-value

  sharat87/just-a-calendar:
    delete_unspecified: false
    secrets:
      ANOTHER_SECRET:
        value: value1
      SOME_LONG_SSH_KEY:
        value: |
          some long ssh key value
          here that goes on for
          several lines
          and then some
```

Now run the following in the folder where `secrets.yml` is located, and all secrets in the specified repositories will be updated to the specified values.

```sh
GITHUB_API_TOKEN=myawesometoken gass sync
```

The `GITHUB_API_TOKEN` is a Personal Access Token (PAT), with at least the `repo` scope.

Additionally, instead of providing the secret values directly in the YAML file, you can get it from the environment by specifying the secret value like this:

```yaml
SECRET_NAME:
  from_env: SECRET_VALUE_ENV_NAME
```

Note that since GitHub doesn't let us see the current value of a secret, we have to update all secret values to ensure they are correct. So the last updated time of all secrets will change every time this program is run, and will also be more-or-less the same.

Keep your `secrets.yml` file **safe**. This is no joke.

## Features

1. Set all repository secrets and organisation secrets with a single command run.
1. Dry run support (`--dry`), that'll only show what will be done, but won't actually do any _write_ API calls.
1. Specify secret values directly as plain text in the YAML file, or give the name of env variable that `gass` will read from.
1. Configuration file is YAML so anchors and aliases can be used, if needed/interested.
1. Detects what secrets are being used in the repository's workflows and prevents deleting any secret that's currently being used.
    1. Also lists secrets that are being used, but aren't specified in the YAML file.

## Roadmap

1. Tests.
1. Better error reporting.
1. Bug fixes.

Yes, that's all. I don't intend to add a lot of new features to this, and that's a feature.

## Tips

This simple tech can be surprisingly useful.

### Using Anchors

The key `vars`, if included at the top-level, will be completely ignored by `gass`. In the below examples, we define it as a _list_, but it could be a map or anything else. It just has to be valid YAML, the actual value is ignored.

We can use YAML Anchors to create something like this:

```yaml
vars:  # Ignored by gass.
  - &artifacts_aws_key "abcdef-key-id"
  - &artifacts_aws_secret "abcdef-secret-key"

repos:

  sharat87/prestige:
    delete_unspecified: true
    secrets:
      AWS_ACCESS_KEY_ID:
        value: *artifacts_aws_key
      AWS_SECRET_ACCESS_KEY:
        value: *artifacts_aws_secret
      SOME_OTHER_SECRET:
        value: "a-super-awesome-secret"

  sharat87/httpbun:
    delete_unspecified: true
    secrets:
      AWS_ACCESS_KEY_ID:
        value: *artifacts_aws_key
      AWS_SECRET_ACCESS_KEY:
        value: *artifacts_aws_secret
```

This will work as you can guess. The parts `*artifacts_aws_...` will be replaced by their corresponding value under `vars`. We can actually simplify this even further, at a slight cost of making it a little less readable perhaps:

```yaml
vars:  # Ignored by gass.
  - &artifacts_aws
    AWS_ACCESS_KEY_ID:
      value: "abcdef-key-id"
    AWS_SECRET_ACCESS_KEY:
      value: "abcdef-secret-key"

repos:

  sharat87/prestige:
    delete_unspecified: true
    secrets:
      <<: *artifacts_aws
      SOME_OTHER_SECRET:
        value: "a-super-awesome-secret"

  sharat87/httpbun:
    delete_unspecified: true
    secrets:
      <<: *artifacts_aws
```

This behaves identical to the previous YAML file.

*Note* that this syntax is just standard YAML. There's no special variable handling and replacements done by `gass` today.

### Auto-apply with GitHub Actions

If you keep your secrets in a private GitHub repo (which may be a good/bad idea depending on who you ask), you can use a GitHub Action like the following to auto-sync when there's a change in your secrets file.

```yaml
# /.github/workflows/gass-sync.yml
name: Gass sync

on:
  push:
    branches:
      - main

  workflow_dispatch:

jobs:
  build:
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v3

      - name: Run Gass
        run: |
          wget --no-verbose -O gass https://github.com/sharat87/gass/releases/download/v0.0.1/gass-linux-amd64
          chmod +x gass
          GITHUB_API_TOKEN=${{ secrets.GASS_GITHUB_API_TOKEN }} ./gass sync --file all.yml
```

Just ensure the repo with this workflow has a secret called `GASS_GITHUB_API_TOKEN`, with a valid GitHub token, and you are set.

## Contributing

Judiciously welcome. I'd appreciated if you [opened an issue](https://github.com/sharat87/gass/issues/new/choose) with details of what you wish to contribute, before you put in the work and open a PR. This can avoid wasted effort, and ensure we are aligned on how to solve something so that your PR will go through fewer cycles of code reviews. Thank you for your interest.

## License

[MIT License](https://github.com/sharat87/gass/blob/master/LICENSE).

## Other Options

If you are invested in the AWS ecosystem, then you could maintain your secrets in AWS Secrets Manager, and load them into your GitHub workflows with the [AWS Secrets Manager Action](https://github.com/marketplace/actions/aws-secrets-manager-action). This is probably safer than saving them in a YAML file, depending on your situation.
