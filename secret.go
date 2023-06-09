package main

import (
	"github.com/urfave/cli/v2"
	"strings"
)

func applySecret(ctx *cli.Context) error {
	gitPat := getGitSecret()
	if gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	reqUrl := GetUrlWithQueryParams("", NG_BASE_URL, "v2/secrets", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	printJson(cliCdRequestData)
	secretBody := createTextSecret("Harness Git Pat", GITHUB_SECRET_IDENTIFIER, gitPat)
	resp, err := Post(reqUrl, cliCdRequestData.AuthToken, secretBody, JSON_CONTENT_TYPE)
	if err != nil {
		println("Error creating secrets")
		return nil
	}
	printJson(resp)
	return nil
}

func getGitSecret() string {

	gitPat := ""

	gitPat = TextInput("Enter your git pat: ")

	if gitPat == "" {
		println("Please enter valid git pat: ")
		return ""
	}
	return gitPat
}

func createTextSecret(secretName string, identifier string, secretValue string) HarnessSecret {
	if identifier == "" {
		identifier = strings.ReplaceAll(secretName, " ", "_")
	}
	newSecret := HarnessSecret{Secret: Secret{Type: "SecretText", Name: secretName, Identifier: identifier, Spec: SecretSpec{Value: secretValue, SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	return newSecret
}
