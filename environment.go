package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// Create update Environment
func applyEnvironment(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update a environment using the yaml=",
		getColoredText(filePath, color.FgCyan))
	createOrUpdateEnvURL := GetUrlWithQueryParams("", baseURL, ENVIRONMENT_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))

	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid environment yaml", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "environment", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "environment", "name").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "environment", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "environment", "orgIdentifier").(string))
	envType := valueToString(GetNestedValue(requestBody, "environment", "type").(string))

	//setup payload for Environment create / update
	EnvPayload := HarnessEnvironment{Identifier: identifier, Name: name, Type: envType,
		ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := getEntity(baseURL, fmt.Sprintf("%s/%s", ENVIRONMENT_ENDPOINT, identifier), projectIdentifier, orgIdentifier, map[string]string{})

	var err error
	if !entityExists {
		println("Creating environment with id: ", getColoredText(identifier, color.FgGreen))
		_, err = Post(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully created environment with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found environment with id: ", getColoredText(identifier, color.FgCyan))
		println("Updating environment details....")
		_, err = Put(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully updated environment with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}
	return nil
}

// Delete an existing Environment
func deleteEnvironment(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Environment
func listEnvironment(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
