package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applySecret(ctx *cli.Context) error {
	baseURL := getNGBaseURL(ctx)
	gitPat := ctx.String("token")
	filePath := ctx.String("file")
	gitPat = getGitSecret(gitPat)
	secretType := ctx.String("secret-type")
	requiresFile := isFileTypeSecret(secretType)
	secretIdentifier := getSecretIdentifier(secretType)
	secretName := getSecretName(secretType)
	var secretBody HarnessSecret
	var headers map[string]string
	var err error

	if requiresFile && filePath == "" {
		println(fmt.Sprintf("Secret type %s requires file path to create a valid filetype secret", secretType))
		return nil
	}
	if !requiresFile && gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	if requiresFile {
		headers = make(map[string]string)
		headers["Content-Type"] = "multipart/form-data"
	}

	createUrl := "v2/secrets"
	if requiresFile {
		createUrl = fmt.Sprintf("v2/secrets/%s", "files")
	}
	createSecretURL := GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	updateUrl := fmt.Sprintf("v2/secrets/%s", secretIdentifier)
	if requiresFile {
		updateUrl = fmt.Sprintf("v2/secrets/files/%s", secretIdentifier)
	}
	updateSecretURL := GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	entityExists := getEntity(baseURL, fmt.Sprintf("v2/secrets/%s", secretIdentifier), DEFAULT_PROJECT,
		DEFAULT_ORG, map[string]string{})

	if requiresFile {
		secretBody = createSecret(secretName, secretIdentifier, gitPat, SecretFile)
	} else {
		secretBody = createSecret(secretName, secretIdentifier, gitPat, SecretText)
	}
	if !entityExists {
		println("Creating secret with default id: ", getColoredText(secretIdentifier, color.FgCyan))
		if requiresFile {
			payload, header, _ := readSecretFromPath(filePath, secretBody)

			_, err = Post(createSecretURL,
				cliCdRequestData.AuthToken,
				nil,
				header,
				payload,
			)

		} else {
			_, err = Post(createSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON, nil)
		}
		if err == nil {
			println(getColoredText("Successfully created secret with id= ", color.FgGreen) +
				getColoredText(secretIdentifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found secret with id: ", getColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")
		if requiresFile {
			payload, header, _ := readSecretFromPath(filePath, secretBody)
			_, err = Put(updateSecretURL, cliCdRequestData.AuthToken,
				nil,
				header, payload,
			)

		} else {
			_, err = Put(updateSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON, nil)
		}
		if err == nil {
			println(getColoredText("Successfully updated secretId= ", color.FgGreen) +
				getColoredText(secretIdentifier, color.FgBlue))
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
func isFileTypeSecret(secType string) bool {
	if secType == "gcp" {
		return true
	}
	return false
}
func createSecret(secretName string, identifier string, secretValue string, secretType string) HarnessSecret {
	typeOfSecret := "SecretText"
	if secretType != "" {
		typeOfSecret = secretType
	}
	if identifier == "" {
		identifier = strings.ReplaceAll(secretName, " ", "_")
	}
	newSecret := HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: DEFAULT_PROJECT,
		OrgIdentifier: DEFAULT_ORG, Spec: SecretSpec{SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	if secretType == SecretText {
		newSecret.Spec.Value = secretValue
	}
	return newSecret
}

func getSecretIdentifier(secType string) string {
	secretIdentifier := ""
	switch secType {
	case "aws":
		secretIdentifier = AWS_SECRET_IDENTIFIER
		break
	case "gcp":
		secretIdentifier = GCP_SECRET_IDENTIFIER
		break
	default:
		secretIdentifier = GITHUB_SECRET_IDENTIFIER
		break
	}
	return secretIdentifier
}

func getSecretName(secType string) string {
	secretName := ""
	switch secType {
	case "aws":
		secretName = "Harness AWS Secret"
		break
	case "gcp":
		secretName = "Harness GCP Secret"
		break
	default:
		secretName = "Harness Git Pat"
		break
	}
	return secretName
}
func readSecretFromPath(filePath string, secretSpec HarnessSecret) (*bytes.Buffer, string, error) {

	secretJSONSpec, err := json.Marshal(secretSpec)
	if err != nil {
		panic(err)
	}
	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)

	_ = writer.WriteField("spec", string(secretJSONSpec))

	file, errFile := os.Open(filePath)
	if errFile != nil {
		return nil, "", errFile
	}
	defer file.Close()

	part, errFile := writer.CreateFormFile("file", filepath.Base(filePath))
	if errFile != nil {
		return nil, "", errFile
	}
	_, errCopy := io.Copy(part, file)
	if errCopy != nil {
		return nil, "", errCopy
	}
	errWriter := writer.Close()
	if errWriter != nil {
		return nil, "", errWriter
	}
	return payload, writer.FormDataContentType(), nil
}
