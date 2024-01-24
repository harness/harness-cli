# Contributing to the Harness CLI

Thank you for considering contributing to the Harness CLI! Contributions fall into a couple of categories:

### Bugs and feature requests

To report a bug or request a feature enhancement, create a [GitHub issue](https://github.com/harness/harness-cli/issues).

### Code contributions

To propose a change to the codebase, open a [pull request](https://github.com/harness/harness-cli/pulls). A maintainer will be happy to triage and ask clarifying questions as needed. We also ask that you build and test proposed changes locally, and include any relevant output and/or screenshots in the PR.

## Development

The Harness CLI is a Go module that interacts heavily with the [Harness API](https://apidocs.harness.io/). Most functionality requires authentication to a Harness account (SaaS or self-managed). You can sign up for free account [here](https://app.harness.io/auth/#/signup?utm_source=harness_io&utm_medium=cta&utm_campaign=platform&utm_content=main_nav). 

1. Ensure you have Go [installed](https://go.dev/doc/install) (version >= 1.19). 
1. [Fork](https://github.com/harness/harness-cli/fork) this repository, then [clone](https://docs.github.com/en/repositories/creating-and-managing-repositories/cloning-a-repository) locally.
2. Navigate into the project directory.
  ```shell
   cd harness-cli/
  ```
3. Make your desired changes to the source code.
4. From the `harness-cli/` directory, compile a new executable with your changes.
  ```shell
   go build -o harness
  ```
5. Run and test your changes.
  ```shell
  ./harness [global options] command [command options] [arguments...]
  ```
6. If contributing, push to your fork and [submit a pull request](https://github.com/harness/harness-cli/pulls). Include relevant output and/or screenshots from local tests.
