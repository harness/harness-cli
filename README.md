# harness-cli
Harness CLI for managing Harness entities, from the command line with YAML as input. This CLI is currently in BETA stage.
Current focus is on the entities needed for the CD & GitOps module. Other modules will be covered in the future.

- Uses public [Harness REST APIs](https://apidocs.harness.io/)
- Requires a [Harness API Key](https://developer.harness.io/docs/platform/user-management/add-and-manage-api-keys/) for authenticating with your Harness account. 

# Instructions
Manual install
To install the Harness CLI tool manually, follow these steps:

1. Download the latest release from the GitHub releases page: https://github.com/harness/harness-cli/releases .
   The tool supports MacOS (darwin + (amd64/arm64)), Linux (linux + (amd64/arm64)), and Windows (windows+amd64) platforms, so make sure to download the correct asset for your 
   platform.
2. Extract the downloaded file to a directory of your choice. It is recommended that you move the extracted file to a folder specified in your system's path for ease of use.
3. Run the `harness help` command to verify that the installation was successful.
If you are using macOS, you can move the `harness` file to the /usr/local/bin/ directory by running the following command:
  `mv harness /usr/local/bin/`
   Then, run the harness help command to verify that the installation was successful.

4. To update the CLI to a newer version, run the following command:
   `harness update`
   This will update the tool to the latest version.
