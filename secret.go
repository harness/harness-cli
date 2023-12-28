package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"harness/client"
	"harness/defaults"
	"harness/shared"
	"harness/telemetry"
	. "harness/types"
	"harness/utils"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/fatih/color"
)

func applySecret(ctx *cli.Context) error {
	baseURL := utils.GetNGBaseURL(ctx)
	token := ctx.String("token")
	password := ctx.String("password")
	filePath := ctx.String("file")
	gitPat := getGitSecret(token)
	secretType := ctx.String("secret-type")
	authType := ctx.String("auth-type")
	port := ctx.String("port")
	username := ctx.String("username")
	domain := ctx.String("domain")
	orgIdentifier := ctx.String("org-id")
	projectIdentifier := ctx.String("project-id")
	requiresFile := isFileTypeSecret(secretType)
	secretName := ctx.String("secret-name")
	secretIdentifier := getSecretIdentifier(secretName)
	var secretBody HarnessSecret
	var err error

	if orgIdentifier == "" {
		orgIdentifier = defaults.DEFAULT_ORG
	}
	if projectIdentifier == "" {
		projectIdentifier = defaults.DEFAULT_PROJECT
	}
	if requiresFile && filePath == "" {
		println(fmt.Sprintf("Secret type %s requires file path to create a valid filetype secret", secretType))
		return nil
	}
	if secretName == "" {
		println("Secret name cannot be empty. Please provide --secret-name.")
		return nil
	}
	if !requiresFile && password == "" && gitPat == "" {
		println("Secret cannot be an empty string")
		return nil
	}
	if authType == "" {
		authType = NTLM
	}
	createUrl := defaults.SECRETS_ENDPOINT
	if requiresFile {
		createUrl = fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, "files")
	}
	createSecretURL := utils.GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projectIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	updateUrl := fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)
	if requiresFile {
		updateUrl = fmt.Sprintf(defaults.FILE_SECRETS_ENDPOINT, secretIdentifier)
	}
	updateSecretURL := utils.GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projectIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	entityExists := utils.GetEntity(baseURL, fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), projectIdentifier,
		orgIdentifier, map[string]string{})
	if strings.EqualFold(secretType, SSHKey) {
		if username == "" {
			username = utils.TextInput("Enter valid username:")
		}

		portNumber, portErr := strconv.Atoi(port)

		if portErr != nil {
			fmt.Println("Port should be a valid port number:")
		}
		err = createSSHSecret(orgIdentifier, projectIdentifier, filePath, "", baseURL, portNumber, username, true)
		return nil
	}
	if strings.EqualFold(secretType, WinRM) {
		if username == "" {
			username = utils.TextInput("Enter valid username:")
		}
		if password == "" {
			password = utils.TextInput("Enter valid password:")
		}
		if domain == "" {
			domain = utils.TextInput("Enter valid domain:")
		}

		portNumber, portErr := strconv.Atoi(port)

		if portErr != nil {
			fmt.Println("Port should be a valid port number:")
		}
		err = createWinRMSecret(orgIdentifier, projectIdentifier, "", baseURL, portNumber, username, password, domain, authType)
		return nil
	}
	if requiresFile {
		secretBody = createSecret(orgIdentifier, projectIdentifier, secretName, secretIdentifier, gitPat, SecretFile, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(orgIdentifier, projectIdentifier, secretName, secretIdentifier, gitPat, SecretText, SSHWINRMSecretData{})
	}
	if !entityExists {
		println("Creating secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))
		if requiresFile {
			payload, header, _ := readSecretFromPath(filePath, secretBody)

			_, err = client.Post(createSecretURL,
				shared.CliCdRequestData.AuthToken,
				nil,
				header,
				payload,
			)

		} else {
			fmt.Println("createSecretURL: " + createSecretURL)
			_, err = client.Post(createSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)
		}
		if err == nil {
			println(utils.GetColoredText("Successfully created secret with id= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            SecretText,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			return nil

		}
	} else {
		println("Found secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")
		if requiresFile {
			payload, header, _ := readSecretFromPath(filePath, secretBody)
			_, err = client.Put(updateSecretURL, shared.CliCdRequestData.AuthToken,
				nil,
				header, payload,
			)

		} else {
			_, err = client.Put(updateSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)
		}
		if err == nil {
			println(utils.GetColoredText("Successfully updated secretId= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            SecretText,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			return nil
		}
	}
	return nil
}

func getGitSecret(userVal string) string {
	gitPat := ""
	if userVal != defaults.GITHUB_PAT_PLACEHOLDER {
		return userVal
	}
	gitPat = utils.TextInput("Enter your git pat: ")

	if gitPat == "" {
		println("Please enter valid git pat: ")
		return ""
	}
	return gitPat
}
func isFileTypeSecret(secretType string) bool {
	switch {
	case strings.EqualFold(secretType, defaults.GCP):
		return true
	case strings.EqualFold(secretType, SSHKey):
		return true
	default:
		return false
	}

}
func createSecret(orgIdentifier string, projectIdentifier string,
	secretName string, identifier string, secretValue string,
	secretType string, secretData SSHWINRMSecretData) HarnessSecret {
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
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret,
			Name:              secretName,
			Identifier:        identifier,
			ProjectIdentifier: projectIdentifier,
			OrgIdentifier:     orgIdentifier,
			Spec:              secretTypeData,
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
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: defaults.DEFAULT_PROJECT,
			OrgIdentifier: orgIdentifier, Spec: secretTypeData,
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
		newSecret = HarnessSecret{Secret: Secret{Type: typeOfSecret, Name: secretName, Identifier: identifier, ProjectIdentifier: projectIdentifier,
			OrgIdentifier: orgIdentifier, Spec: SecretSpec{SecretManagerIdentifier: "harnessSecretManager", ValueType: "Inline"}}}
	}

	if spec, ok := newSecret.Spec.(SecretSpec); ok {
		spec.Value = secretValue
		newSecret.Spec = spec
	}
	return newSecret
}

func getSecretIdentifier(secName string) string {
	secretIdentifier := ""
        reCleaner := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	secretIdentifier = reCleaner.ReplaceAllString(secName, "")
	return secretIdentifier
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

func createSSHSecret(orgIdentifier string, projIdentifier string, filepath string,
	secretIdentifier string, baseURL string, port int, username string, requiresFile bool) error {
	var err error
	var secretBody HarnessSecret
	secretTypeName := SecretText
	isSSHFileSecret := secretIdentifier == ""
	if isSSHFileSecret {
		secretIdentifier = defaults.SSH_KEY_FILE_SECRET_IDENTIFIER
		secretTypeName = SecretFile
	}
	createUrl := defaults.SECRETS_ENDPOINT
	if requiresFile {
		createUrl = fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, "files")
	}
	createSecretURL := utils.GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	updateUrl := fmt.Sprintf(defaults.FILE_SECRETS_ENDPOINT, secretIdentifier)
	updateSSHSecretURL := fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)
	updateSecretURL := utils.GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	updatedSSHSecretURL := utils.GetUrlWithQueryParams("", baseURL, updateSSHSecretURL, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	fileSecretExists := utils.GetEntity(baseURL, fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), projIdentifier,
		orgIdentifier, map[string]string{})
	if isSSHFileSecret {
		secretBody = createSecret(orgIdentifier, projIdentifier, secretIdentifier, secretIdentifier, "", SecretFile, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(orgIdentifier, projIdentifier, secretIdentifier, secretIdentifier, "", SSHKey, SSHWINRMSecretData{Port: port, Username: username, Key: defaults.SSH_KEY_FILE_SECRET_IDENTIFIER})
	}
	if !fileSecretExists {
		println("Creating secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))
		if isSSHFileSecret {
			payload, header, _ := readSecretFromPath(filepath, secretBody)

			_, err = client.Post(createSecretURL,
				shared.CliCdRequestData.AuthToken,
				nil,
				header,
				payload,
			)

		} else {

			_, err = client.Post(createSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)
		}
		if err == nil {
			println(utils.GetColoredText("Successfully created secret with id= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            secretTypeName,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
		}

	} else {
		println("Found secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")
		if isSSHFileSecret {

			payload, header, _ := readSecretFromPath(filepath, secretBody)
			_, err = client.Put(updateSecretURL, shared.CliCdRequestData.AuthToken,
				nil,
				header, payload,
			)

		} else {

			_, err = client.Put(updatedSSHSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)

		}
		if err == nil {
			println(utils.GetColoredText("Successfully updated secretId= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            secretTypeName,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
		}
	}
	if secretIdentifier == defaults.SSH_PRIVATE_KEY_SECRET_IDENTIFIER {
		return nil
	}
	return createSSHSecret(orgIdentifier, projIdentifier, "", defaults.SSH_PRIVATE_KEY_SECRET_IDENTIFIER, baseURL, port, username, false)
}

func createWinRMSecret(orgIdentifier string, projIdentifier string, secretIdentifier string, baseURL string, port int, username string, password string, domain string, authType string) error {
	var err error
	var secretBody HarnessSecret

	isWinRMPasswordSecret := secretIdentifier == ""
	if isWinRMPasswordSecret {
		secretIdentifier = defaults.WINRM_PASSWORD_SECRET_IDENTIFIER
	}
	createUrl := defaults.SECRETS_ENDPOINT

	createSecretURL := utils.GetUrlWithQueryParams("", baseURL, createUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	updateUrl := fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier)

	updateSecretURL := utils.GetUrlWithQueryParams("", baseURL, updateUrl, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"projectIdentifier": projIdentifier,
		"orgIdentifier":     orgIdentifier,
	})

	secretExists := utils.GetEntity(baseURL, fmt.Sprintf(defaults.SECRETS_ENDPOINT_WITH_IDENTIFIER, secretIdentifier), projIdentifier,
		orgIdentifier, map[string]string{})
	if isWinRMPasswordSecret {
		secretBody = createSecret(orgIdentifier, projIdentifier, secretIdentifier, secretIdentifier, password, SecretText, SSHWINRMSecretData{})
	} else {
		secretBody = createSecret(orgIdentifier, projIdentifier, secretIdentifier, secretIdentifier, "", WinRM, SSHWINRMSecretData{Port: port, Username: username, Password: defaults.WINRM_PASSWORD_SECRET_IDENTIFIER, Domain: domain, AuthType: authType})
	}
	if !secretExists {
		println("Creating secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))

		_, err = client.Post(createSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(utils.GetColoredText("Successfully created secret with id= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            WinRM,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})

		}
	} else {
		println("Found secret with id: ", utils.GetColoredText(secretIdentifier, color.FgCyan))
		println("Updating secret details....")

		_, err = client.Put(updateSecretURL, shared.CliCdRequestData.AuthToken, secretBody, defaults.CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(utils.GetColoredText("Successfully updated secretId= ", color.FgGreen) +
				utils.GetColoredText(secretIdentifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SECRET_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            WinRM,
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})

		}
	}
	if secretIdentifier == defaults.WINRM_SECRET_IDENTIFIER {
		return nil
	}
	return createWinRMSecret(orgIdentifier, projIdentifier, defaults.WINRM_SECRET_IDENTIFIER, baseURL, port, username, password, domain, authType)
}
