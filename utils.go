package main

import (
	"encoding/json"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

func ConfirmInput(question string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: question,
	}
	_ = survey.AskOne(prompt, &confirm)
	return confirm
}

func TextInput(question string) string {
	var text = ""
	prompt := &survey.Input{
		Message: question,
	}
	err := survey.AskOne(prompt, &text, survey.WithValidator(survey.Required))
	if err != nil {
		log.Error(err.Error())
		os.Exit(0)
	}
	return text
}

func GetUrlWithQueryParams(environment string, service string, endpoint string, queryParams map[string]string) string {
	params := ""
	for k, v := range queryParams {
		params = params + k + "=" + v + "&"
	}

	fmt.Println("baseUrl", cliCdRequestData.BaseUrl)
	//return fmt.Sprintf("%s/api/accounts/%s?%s", cliCdRequestData.BaseUrl, cliCdRequestData.Account, params)
	return fmt.Sprintf("%s/api/%s/?%s", "https://app.harness.io/gateway/ng", endpoint, params)
}

func printJson(v any) {
	marsheld, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		log.Fatalf("Marshaling error: %s", err)
	}
	fmt.Println("Data: ", string(marsheld))
}

func writeToFile(text string, filepath string, overwrite bool) {
	var permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if overwrite {
		permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(filepath, permissions, 0644)
	if overwrite {
		f.WriteString("")
	}
	f.WriteString(text)
	if err != nil {
		log.Fatal(err)
	}

	f.Close()
}

func readFromFile(filepath string) (s string) {
	var _fileContents = ""
	buffer := make([]byte, 1024)
	file, fileError := os.OpenFile(filepath, os.O_RDONLY, 0644)
	defer file.Close()
	for {
		reader, readError := file.Read(buffer)
		if readError != nil {
			if readError == io.EOF {
				break
			} else {
				log.Println("Error reading from file:", fileError)
				break
			}
		}
		_fileContents = string(buffer[:reader])
	}
	return _fileContents
}

func saveCredentials() (err error) {
	authCredentials := SecretStore{
		ApiKey:    cliCdRequestData.AuthToken,
		AccountId: cliCdRequestData.Account,
	}
	jsonObj, err := json.MarshalIndent(authCredentials, "", "  ")
	if err != nil {
		fmt.Println("Error creating secrets json:", err)
		return
	}

	writeToFile(string(jsonObj), SECRETS_STORE_PATH, true)
	return nil
}
func hydrateCredsFromPersistence(c *cli.Context) {
	if cliCdRequestData.AuthToken != "" && cliCdRequestData.Account != "" {
		return
	}
	credsJson, err := os.ReadFile(SECRETS_STORE_PATH)
	if err != nil {
		fmt.Println("Error reading creds file:", err)
		return
	}
	var secretstore SecretStore
	err = json.Unmarshal(credsJson, &secretstore)
	if err != nil {
		fmt.Println("Error:", err)
		Login(c)
		return
	}
	cliCdRequestData.AuthToken = secretstore.ApiKey
	cliCdRequestData.Account = secretstore.AccountId
	return
}

func getJsonFromYaml(content string) (requestBody map[string]interface{}) {
	//respObj := &map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return nil
	}

	return requestBody
}
