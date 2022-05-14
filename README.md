# GitHub Actions Secrets Sync (`gass`) <sup>Beta</sup>

A tiny tool to set secrets for one or more GitHub repos, as specified in a YAML file. For example, consider a `secrets.yml` with:

```yaml
repos:
  - owner: sharat87
    name: prestige
    delete_unspecified: false  # If true, delete any secrets not mentioned in the below `secrets` list. Defaults to `false` if not specified.
    secrets:
      SOME_SECRET_NAME: super-secret-value

  - owner: sharat87
    name: just-a-calendar
    delete_unspecified: false
    secrets:
      ANOTHER_SECRET: value1
      SOME_LONG_SSH_KEY: |
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
  fromEnv: SECRET_VALUE_ENV_NAME
```

Note that since GitHub doesn't let us see the current value of a secret, we have to update all secret values to ensure they are correct. So the last updated time of all secrets will change every time this program is run, and will also be more-or-less the same.

Keep your `secrets.yml` file **safe**. This is no joke.

## Features

1. Set all repository secrets, with a single command run.
1. Dry run support, that'll only show what will be done, but won't actually do any _write_ API calls.
1. Specify secret values directly as plain text in the YAML file, or give the name of env variable that `gass` will read from.
1. Configuration file is YAML so anchors and aliases can be used, if needed/interested.
1. Detects what secrets are being used in the repository's workflows and prevents deleting any secret that's currently being used.
    1. Also lists secrets that are being used, but aren't specified in the YAML file.

## Roadmap

I don't intend to add a lot flexibility and features to this, and that's deliberate, conscious, and is treated as a feature. The following are things that _may_ happen in the future, when I find the time or if there's significant interest from the community.

1. Tests.
1. Support organization secrets.
1. Support environment secrets.

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

  - owner: sharat87
    name: prestige
    delete_unspecified: true
    secrets:
      AWS_ACCESS_KEY_ID: *artifacts_aws_key
      AWS_SECRET_ACCESS_KEY: *artifacts_aws_secret
      SOME_OTHER_SECRET: "a-super-awesome-secret"

  - owner: sharat87
    name: httpbun
    delete_unspecified: true
    secrets:
      AWS_ACCESS_KEY_ID: *artifacts_aws_key
      AWS_SECRET_ACCESS_KEY: *artifacts_aws_secret
```

This will work as you can guess. The parts `*artifacts_aws_...` will be replaced by their corresponding value under `vars`. We can actually simplify this even further, at a slight cost of making it a little less readable perhaps:

```yaml
vars:  # Ignored by gass.
  - &artifacts_aws
    AWS_ACCESS_KEY_ID: "abcdef-key-id"
    AWS_SECRET_ACCESS_KEY: "abcdef-secret-key"

repos:

  - owner: sharat87
    name: prestige
    delete_unspecified: true
    secrets:
      <<: *artifacts_aws
      SOME_OTHER_SECRET: "a-super-awesome-secret"

  - owner: sharat87
    name: httpbun
    delete_unspecified: true
    secrets:
      <<: *artifacts_aws
```

This behaves identical to the previous YAML file.

*Note* that this syntax is just standard YAML. There's no special variable handling and replacements done by `gass` today.

### Auto-apply with GitHub Actions

WIP

## Contributing

Judiciously welcome. I'd appreciated if you [opened an issue](https://github.com/sharat87/gass/issues/new/choose) with details of what you wish to contribute, before you put in the work and open a PR. This can avoid wasted effort, and ensure we are aligned on how to solve something so that your PR will go through fewer cycles of code reviews. Thank you for your interest.

## License

[MIT License](https://github.com/sharat87/gass/blob/master/LICENSE).
