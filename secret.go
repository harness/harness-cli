package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"strings"
)

func applySecret(ctx *cli.Context) error {
	gitPat := ctx.String("token")

	if gitPat == "" {
		gitPat = getGitSecret()
	}
	if gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	createSecretURL := GetUrlWithQueryParams("", NG_BASE_URL, "v2/secrets", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	updateSecretURL := GetUrlWithQueryParams("", NG_BASE_URL, fmt.Sprintf("v2/secrets/%s", GITHUB_SECRET_IDENTIFIER), map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	entityExists := getEntity(NG_BASE_URL, fmt.Sprintf("v2/secrets/%s", GITHUB_SECRET_IDENTIFIER), DEFAULT_PROJECT, DEFAULT_ORG, map[string]string{})

	secretBody := createTextSecret("Harness Git Pat", GITHUB_SECRET_IDENTIFIER, gitPat)
	var resp ResponseBody
	var err error
	if !entityExists {
		println("Creating secret with id: ", getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgGreen))
		resp, err = Post(createSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Secret created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found secret with id: ", getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgGreen))

		println(getColoredText("Updating secret details....", color.FgGreen))
		resp, err = Put(updateSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Secret updated successfully!", color.FgGreen))
			//printJson(resp.Data)
			return nil
		}
	}
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
