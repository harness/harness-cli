# Installing the Harness CLI by downloading the binary

You can install the Harness CLI (`harness`) in order to interact with Harness Platform from a command-line interface. You can install `harness` on Linux, Windows, or macOS.

## Table of Contents

   * [Installing the CLI on Linux](#installing-the-cli-on-linux)
   * [Installing the CLI on Windows](#installing-the-cli-on-windows)
   * [Installing the CLI on macOS](#installing-the-cli-on-macos)

## Installing the CLI on Linux

1. Navigate to [Harness CLI Releases](https://github.com/harness/harness-cli/tags) page.

2. Click the release version (recommended: `latest`), your linux architecture type, and, download the archive like in the below example:
```bash
curl -LOJ https://github.com/harness/harness-cli/releases/download/<VERSION>/harness-<VERSION>-linux-<ARCH>.tar.gz
```
> NOTE: Replace `VERSION` and `ARCH` with the release version and architecture type.

3. Unpack the archive:
```bash
tar xvzf harness-<VERSION>-linux-<ARCH>.tar.gz
```

4. Move the `harness` binary in a directory that is on your `PATH`.
    - To check your `PATH`, execute the following command:
        ```bash
        echo $PATH
        ```
    - To move the `harness` binary
        ```bash
        mv ./harness /PATH/TO/DEST
        ```

5. After you install the CLI, it is available using the `harness` command:
```bash
harness harness --version
```

## Installing the CLI on Windows
[TODO]

## Installing the CLI on macOS
[TODO]
