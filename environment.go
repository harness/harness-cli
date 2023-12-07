package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// Create update Environment
func applyEnvironment(c *cli.Context) error {
	filePath := c.String("file")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")
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
	if projectIdentifier != "" {
		content = ReplacePlaceholderValues(content, defaults.DEFAULT_PROJECT, projectIdentifier)
	}
	if orgIdentifier != "" {
		content = ReplacePlaceholderValues(content, defaults.DEFAULT_ORG, orgIdentifier)
	}

	requestBody := GetJsonFromYaml(content)
	if requestBody == nil {
		println(GetColoredText("Please enter valid environment yaml", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "environment", "identifier").(string))
	name := ValueToString(GetNestedValue(requestBody, "environment", "name").(string))

	// Check if the project and org values are provided by the user otherwise default them
	if projectIdentifier == "" {
		projectIdentifier = ValueToString(GetNestedValue(requestBody, "environment", "projectIdentifier").(string))
	}
	if orgIdentifier == "" {
		orgIdentifier = ValueToString(GetNestedValue(requestBody, "environment", "orgIdentifier").(string))
	}
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
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.ENV_UPDATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
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
