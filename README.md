# GitHub Actions Secrets Sync (`gass`)

A tiny tool to set secrets for one or more GitHub repos, as specified in a YAML file. For example, consider a `secrets.yml` with:

```yaml
repos:

  - owner: sharat87
    name: prestige
    deleteUnspecified: true  # If true, delete any secrets not mentioned in the below `secrets` list.
    secrets:
      SOME_SECRET_NAME: super-secret-value

  - owner: sharat87
    name: just-a-calendar
    deleteUnspecified: false
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
GITHUB_API_TOKEN=myawesometoken gass
```

The `GITHUB_API_TOKEN` is a Personal Access Token (PAT), with at least the `repo` scope.

That's it. Keep your `secrets.yml` file **safe**. This is no joke.

## Roadmap

I don't intend to add a lot flexibility and features to this, and that's deliberate, conscious, and is treated as a feature. The following are things that _may_ happen in the future, when I find the time or if there's significant interest from the community.

1. Support syncing organization secrets.
1. Take secrets file from command line. Something like `-f secrets.yml`.
1. Support for dry-run?

## Contributing

Cautiously welcome. I'd appreciated if you [opened an issue](https://github.com/sharat87/gass/issues/new/choose) with details of what you wish to contribute, before you put in the work and open a PR. This can avoid wasted effort, and ensure we are aligned on how to solve something so that your PR will go through fewer cycles of code reviews. Thank you for your interest.

## License

[MIT License](https://github.com/sharat87/gass/blob/master/LICENSE).
