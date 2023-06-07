package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"strings"
)

func applySecret(ctx *cli.Context) error {
	gitPat := getGitSecret(ctx)
	if gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	reqUrl := GetUrlWithQueryParams("", "", "v2/secrets", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	printJson(cliCdRequestData)
	secretBody := createTextSecret("Harness Git Pat", "harness_gitpat", gitPat)
	resp, err := Post(reqUrl, cliCdRequestData.AuthToken, secretBody, JSON_CONTENT_TYPE)
	if err != nil {
		println("Error creating secrets")
		return nil
	}
	printJson(resp)
	return nil
}

func getGitSecret(ctx *cli.Context) string {

	gitPat := ""

	gitPat = TextInput("Enter your git pat: ")

	if gitPat == "" {
		println("Please enter valid git pat: ")
		return ""
	}
	fmt.Printf("You entered gitpat: %s", gitPat)
	return gitPat
}

func createTextSecret(secretName string, identifier string, secretValue string) HarnessSecret {
	if identifier == "" {
		identifier = strings.ReplaceAll(secretName, " ", "_")
	}
	newSecret := HarnessSecret{Secret: Secret{Type: "SecretText", Name: secretName, Identifier: identifier, Spec: SecretSpec{Value: secretValue, SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	return newSecret
}
