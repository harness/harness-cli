package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applySecret(ctx *cli.Context) error {
	baseURL := getNGBaseURL(ctx)
	token := ctx.String("token")
	password := ctx.String("password")
	filePath := ctx.String("file")
	gitPat := getGitSecret(token)
	secretType := ctx.String("secret-type")
	authType := ctx.String("auth-type")
	port := ctx.String("port")
	username := ctx.String("username")
	domain := ctx.String("domain")
	requiresFile := isFileTypeSecret(secretType)
	secretIdentifier := getSecretIdentifier(secretType)
	secretName := getSecretName(secretType)
	var secretBody HarnessSecret
	var err error

	if requiresFile && filePath == "" {
		println(fmt.Sprintf("Secret type %s requires file path to create a valid filetype secret", secretType))
		return nil
	}
	if !requiresFile && password == "" && gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	if authType == "" {
		authType = NTLM
	}
	createUrl := SECRETS_ENDPOINT
	if requiresFile {
		createUrl = fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, "files")
	}
	createSecretURL := GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	updateUrl := fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)
	if requiresFile {
		updateUrl = fmt.Sprintf(FILE_SECRETS_ENDPOINT, secretIdentifier)
	}
	updateSecretURL := GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	entityExists := getEntity(baseURL, fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), DEFAULT_PROJECT,
		DEFAULT_ORG, map[string]string{})
	if strings.EqualFold(secretType, SSHKey) {
		if username == "" {
			username = TextInput("Enter valid username:")
		}

		portNumber, portErr := strconv.Atoi(port)

		if portErr != nil {
			fmt.Println("Port should be a valid port number:")
		}
		err = createSSHSecret(filePath, "", baseURL, portNumber, username, true)
		return nil
	}
	if strings.EqualFold(secretType, WinRM) {
		if username == "" {
			username = TextInput("Enter valid username:")
		}
		if password == "" {
			password = TextInput("Enter valid password:")
		}
		if domain == "" {
			domain = TextInput("Enter valid domain:")
		}

		portNumber, portErr := strconv.Atoi(port)

		if portErr != nil {
			fmt.Println("Port should be a valid port number:")
		}
		err = createWinRMSecret("", baseURL, portNumber, username, password, domain, authType)
		return nil
	}
	if requiresFile {
		secretBody = createSecret(secretName, secretIdentifier, gitPat, SecretFile, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(secretName, secretIdentifier, gitPat, SecretText, SSHWINRMSecretData{})
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
func isFileTypeSecret(secretType string) bool {
	switch {
	case strings.EqualFold(secretType, GCP):
		return true
	case strings.EqualFold(secretType, SSHKey):
		return true
	default:
		return false
	}

}
func createSecret(secretName string, identifier string, secretValue string, secretType string, secretData SSHWINRMSecretData) HarnessSecret {
	typeOfSecret := "SecretText"

	var newSecret HarnessSecret
	if secretType != "" {
		typeOfSecret = secretType
	}
	if identifier == "" {
		identifier = strings.ReplaceAll(secretName, " ", "_")
	}

	if strings.EqualFold(secretType, SSHKey) {

		secretTypeData := SSHSecretType{
			Auth: SecretAuth{
				Type: SShSecretType,
				Spec: SSHSecretSpec{
					CredentialType: "KeyReference",
					Spec: SShSecretSubSpec{
						UserName: "",
						Key:      "",
					},
				},
			},
			Port: 22,
		}
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: DEFAULT_PROJECT,
			OrgIdentifier: DEFAULT_ORG, Spec: secretTypeData,
		}}

		if sshspec, ok := newSecret.Spec.(SSHSecretType); ok {
			sshspec.Port = secretData.Port
			sshspec.Auth.Spec.Spec.UserName = secretData.Username
			sshspec.Auth.Spec.Spec.Key = secretData.Key

			newSecret.Spec = sshspec
		}
	} else if strings.EqualFold(secretType, WinRM) {

		secretTypeData := WinRMSecretType{
			Auth: WinRMSecretAuth{
				Type: NTLM,
				Spec: WinRMSecretSpec{
					Username: "",
					Password: "",
					Domain:   "",
				},
			},
			Port:       5985,
			Parameters: []string{},
		}
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: DEFAULT_PROJECT,
			OrgIdentifier: DEFAULT_ORG, Spec: secretTypeData,
		}}

		if winrmSpec, ok := newSecret.Spec.(WinRMSecretType); ok {
			winrmSpec.Port = secretData.Port
			winrmSpec.Auth.Type = secretData.AuthType
			winrmSpec.Auth.Spec.Username = secretData.Username
			winrmSpec.Auth.Spec.Password = secretData.Password
			winrmSpec.Auth.Spec.Domain = secretData.Domain

			newSecret.Spec = winrmSpec
		}
	} else {
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: DEFAULT_PROJECT,
			OrgIdentifier: DEFAULT_ORG, Spec: SecretSpec{SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	}

	if spec, ok := newSecret.Spec.(SecretSpec); ok {
		spec.Value = secretValue
		newSecret.Spec = spec
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

func getSecretName(secretType string) string {
	secretName := ""
	switch {
	case strings.EqualFold(secretType, AWS):
		secretName = "Harness AWS Secret"
		break
	case strings.EqualFold(secretType, GCP):
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

func createSSHSecret(filepath string, secretIdentifier string, baseURL string, port int, username string, requiresFile bool) error {
	var err error
	var secretBody HarnessSecret

	isSSHFileSecret := secretIdentifier == ""
	if isSSHFileSecret {
		secretIdentifier = SSH_KEY_FILE_SECRET_IDENTIFIER
	}
	createUrl := SECRETS_ENDPOINT
	if requiresFile {
		createUrl = fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, "files")
	}
	createSecretURL := GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	updateUrl := fmt.Sprintf(FILE_SECRETS_ENDPOINT, secretIdentifier)
	updateSSHSecretURL := fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)
	updateSecretURL := GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	updatedSSHSecretURL := GetUrlWithQueryParams("", baseURL, updateSSHSecretURL, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	fileSecretExists := getEntity(baseURL, fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), DEFAULT_PROJECT,
		DEFAULT_ORG, map[string]string{})
	if isSSHFileSecret {
		secretBody = createSecret(secretIdentifier, secretIdentifier, "", SecretFile, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(secretIdentifier, secretIdentifier, "", SSHKey, SSHWINRMSecretData{Port: port, Username: username, Key: SSH_KEY_FILE_SECRET_IDENTIFIER})
	}
	if !fileSecretExists {
		println("Creating secret with default id: ", getColoredText(secretIdentifier, color.FgCyan))
		if isSSHFileSecret {
			payload, header, _ := readSecretFromPath(filepath, secretBody)

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

		}
	} else {
		println("Found secret with id: ", getColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")
		if isSSHFileSecret {

			payload, header, _ := readSecretFromPath(filepath, secretBody)
			_, err = Put(updateSecretURL, cliCdRequestData.AuthToken,
				nil,
				header, payload,
			)

		} else {

			_, err = Put(updatedSSHSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON, nil)

		}
		if err == nil {
			println(getColoredText("Successfully updated secretId= ", color.FgGreen) +
				getColoredText(secretIdentifier, color.FgBlue))

		}
	}
	if secretIdentifier == SSH_PRIVATE_KEY_SECRET_IDENTIFIER {
		return nil
	}
	return createSSHSecret("", SSH_PRIVATE_KEY_SECRET_IDENTIFIER, baseURL, port, username, false)
}

func createWinRMSecret(secretIdentifier string, baseURL string, port int, username string, password string, domain string, authType string) error {
	var err error
	var secretBody HarnessSecret

	isWinRMPasswordSecret := secretIdentifier == ""
	if isWinRMPasswordSecret {
		secretIdentifier = WINRM_PASSWORD_SECRET_IDENTIFIER
	}
	createUrl := SECRETS_ENDPOINT

	createSecretURL := GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	updateUrl := fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)

	updateSecretURL := GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"projectIdentifier": DEFAULT_PROJECT,
		"orgIdentifier":     DEFAULT_ORG,
	})

	secretExists := getEntity(baseURL, fmt.Sprintf(SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), DEFAULT_PROJECT,
		DEFAULT_ORG, map[string]string{})
	if isWinRMPasswordSecret {
		secretBody = createSecret(secretIdentifier, secretIdentifier, password, SecretText, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(secretIdentifier, secretIdentifier, "", WinRM, SSHWINRMSecretData{Port: port, Username: username, Password: WINRM_PASSWORD_SECRET_IDENTIFIER, Domain: domain, AuthType: authType})
	}
	if !secretExists {
		println("Creating secret with default id: ", getColoredText(secretIdentifier, color.FgCyan))

		_, err = Post(createSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(getColoredText("Successfully created secret with id= ", color.FgGreen) +
				getColoredText(secretIdentifier, color.FgBlue))

		}
	} else {
		println("Found secret with id: ", getColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")

		_, err = Put(updateSecretURL, cliCdRequestData.AuthToken, secretBody, CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(getColoredText("Successfully updated secretId= ", color.FgGreen) +
				getColoredText(secretIdentifier, color.FgBlue))

		}
	}
	if secretIdentifier == WINRM_SECRET_IDENTIFIER {
		return nil
	}
	return createWinRMSecret(WINRM_SECRET_IDENTIFIER, baseURL, port, username, password, domain, authType)
}
