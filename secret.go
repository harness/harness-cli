package main

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applySecret(ctx *cli.Context) error {
	baseURL := getNGBaseURL(ctx)
	gitPat := ctx.String("token")
	gitPat = getGitSecret(gitPat)

	if gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	createSecretURL := GetUrlWithQueryParams("", baseURL, "v2/secrets", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})
	updateSecretURL := GetUrlWithQueryParams("", baseURL, fmt.Sprintf("v2/secrets/%s", GITHUB_SECRET_IDENTIFIER), map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})
	entityExists := getEntity(baseURL, fmt.Sprintf("v2/secrets/%s", GITHUB_SECRET_IDENTIFIER), DEFAULT_PROJECT,
		DEFAULT_ORG, map[string]string{})

	secretBody := createTextSecret("Harness Git Pat", GITHUB_SECRET_IDENTIFIER, gitPat)
	var err error
	if !entityExists {
		println("Creating secret with default id: ", getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgCyan))
		_, err = Post(createSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully created secret with id= ", color.FgGreen) +
				getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgBlue))
			return nil
		}
	} else {
		println("Found secret with id: ", getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgCyan))
		println("Updating secret details....")
		_, err = Put(updateSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully updated secretId= ", color.FgGreen) +
				getColoredText(GITHUB_SECRET_IDENTIFIER, color.FgBlue))
			return nil
		}
	}
	return nil
}

func getGitSecret(userVal string) string {
	gitPat := ""
	if userVal != GITHUB_PAT_PLACEHOLDER {
		return userVal
	}
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
	newSecret := HarnessSecret{Secret: Secret{Type: "SecretText", Name: secretName, Identifier: identifier, ProjectIdentifier: DEFAULT_PROJECT,
		OrgIdentifier: DEFAULT_ORG, Spec: SecretSpec{Value: secretValue, SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	return newSecret
}
