package main

import (
	"encoding/json"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	log "github.com/sirupsen/logrus"
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

	fmt.Println("baseUrl", cliCdReq.BaseUrl)
	//return fmt.Sprintf("%s/api/accounts/%s?%s", cliCdReq.BaseUrl, cliCdReq.Account, params)
	return fmt.Sprintf("%s/api/accounts/%s?%s", "https://app.harness.io/gateway/ng", cliCdReq.Account, params)
}

func printJson(v any) {
	marsheld, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		log.Fatalf("Marshaling error: %s", err)
	}
	fmt.Println("Data: ", string(marsheld))
}
