package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
)

// Create update Environment
func applyEnvironment(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := GetNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update a environment using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	createOrUpdateEnvURL := GetUrlWithQueryParams("", baseURL, defaults.ENVIRONMENT_ENDPOINT, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})
	var content, _ = ReadFromFile(c.String("file"))

	requestBody := GetJsonFromYaml(content)
	if requestBody == nil {
		println(GetColoredText("Please enter valid environment yaml", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "environment", "identifier").(string))
	name := ValueToString(GetNestedValue(requestBody, "environment", "name").(string))
	projectIdentifier := ValueToString(GetNestedValue(requestBody, "environment", "projectIdentifier").(string))
	orgIdentifier := ValueToString(GetNestedValue(requestBody, "environment", "orgIdentifier").(string))
	envType := ValueToString(GetNestedValue(requestBody, "environment", "type").(string))

	//setup payload for Environment create / update
	EnvPayload := HarnessEnvironment{Identifier: identifier, Name: name, Type: envType,
		ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := GetEntity(baseURL, fmt.Sprintf("%s/%s", defaults.ENVIRONMENT_ENDPOINT, identifier), projectIdentifier, orgIdentifier, map[string]string{})

	var err error
	if !entityExists {
		println("Creating environment with id: ", GetColoredText(identifier, color.FgGreen))
		_, err = client.Post(createOrUpdateEnvURL, CliCdRequestData.AuthToken, EnvPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created environment with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.ENV_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    CliCdRequestData.UserId,
			})
			return nil
		}
	} else {
		println("Found environment with id: ", GetColoredText(identifier, color.FgCyan))
		println("Updating environment details....")
		_, err = client.Put(createOrUpdateEnvURL, CliCdRequestData.AuthToken, EnvPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully updated environment with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.ENV_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    CliCdRequestData.UserId,
			})
			return nil
		}
	}
	return nil
}

// Delete an existing Environment
func deleteEnvironment(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Environment
func listEnvironment(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}
