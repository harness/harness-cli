package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applyService(c *cli.Context) error {
	filePath := c.String("file")
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update service using the yaml=",
		getColoredText(filePath, color.FgCyan))
	createOrUpdateSvcURL := GetUrlWithQueryParams("", NG_BASE_URL, "servicesV2", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))
	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid yaml", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "service", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "service", "name").(string))
	//setup payload for svc create / update
	svcPayload := HarnessService{Identifier: identifier, Name: name,
		ProjectIdentifier: DEFAULT_PROJECT, OrgIdentifier: DEFAULT_ORG, Yaml: content}
	entityExists := getEntity(NG_BASE_URL, fmt.Sprintf("servicesV2/%s", identifier),
		DEFAULT_PROJECT, DEFAULT_ORG, map[string]string{})
	var err error
	if !entityExists {
		println("Creating service with id: ", getColoredText(identifier, color.FgGreen))
		_, err = Post(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully created service with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found service with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of service with id=", getColoredText(identifier, color.FgBlue))
		_, err = Put(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully updated connector with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}

	return nil
}

func deleteService(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
