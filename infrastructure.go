package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	"harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var cloudProjectName = ""
var cloudRegionName = ""
var cloudInstanceName = ""

// create or update  Infra Definition
func applyInfraDefinition(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := GetNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	cloudProjectName = c.String("cloud-project")
	cloudRegionName = c.String("cloud-region")
	cloudInstanceName = c.String("instance-name")

	fmt.Println("Trying to create or update infrastructure using the given yaml=",
		GetColoredText(filePath, color.FgCyan))

	createOrUpdateInfraURL := GetUrlWithQueryParams("", baseURL, defaults.INFRA_ENDPOINT, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
	})
	var content, _ = ReadFromFile(c.String("file"))
	content = updateInfraYamlContent(content)

	requestBody := GetJsonFromYaml(content)
	if requestBody == nil {
		println(GetColoredText("Please enter a valid yaml and try again...", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "infrastructureDefinition", "identifier").(string))
	name := ValueToString(GetNestedValue(requestBody, "infrastructureDefinition", "name").(string))
	projectIdentifier := ValueToString(GetNestedValue(requestBody, "infrastructureDefinition", "projectIdentifier").(string))
	orgIdentifier := ValueToString(GetNestedValue(requestBody, "infrastructureDefinition", "orgIdentifier").(string))
	environmentRef := ValueToString(GetNestedValue(requestBody, "infrastructureDefinition", "environmentRef").(string))
	//setup payload for Infra create / update
	InfraPayload := HarnessInfra{Identifier: identifier, Name: name, ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := GetEntity(baseURL, fmt.Sprintf("infrastructures/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{
			"environmentIdentifier": environmentRef,
		})
	var err error
	if !entityExists {
		println("Creating infrastructure with id: ", GetColoredText(identifier, color.FgGreen))
		_, err = client.Post(createOrUpdateInfraURL, shared.CliCdRequestData.AuthToken, InfraPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Infrastructure Definition created successfully!", color.FgGreen))

			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.INFRA_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": shared.CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    shared.CliCdRequestData.UserId,
			})
			return nil
		}
	} else {
		println("Found infrastructure with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of infrastructure with id=", GetColoredText(identifier, color.FgBlue))
		_, err = client.Put(createOrUpdateInfraURL, shared.CliCdRequestData.AuthToken, InfraPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully updated infrastructure definition with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))

			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.INFRA_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": shared.CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    shared.CliCdRequestData.UserId,
			})
			return nil
		}
	}

	return nil
}

func updateInfraYamlContent(content string) string {
	hasRegionName := strings.Contains(content, defaults.REGION_NAME_PLACEHOLDER)
	hasProjectName := strings.Contains(content, defaults.PROJECT_NAME_PLACEHOLDER)
	hasInstanceName := strings.Contains(content, defaults.INSTANCE_NAME_PLACEHOLDER)

	if hasProjectName && (cloudProjectName == "" || cloudProjectName == defaults.PROJECT_NAME_PLACEHOLDER) {
		cloudProjectName = TextInput("Enter a valid project name:")
	}

	if hasInstanceName && (cloudInstanceName == "" || cloudInstanceName == defaults.INSTANCE_NAME_PLACEHOLDER) {
		cloudInstanceName = TextInput("Enter a valid instance name:")
	}

	if hasRegionName && (cloudRegionName == "" || cloudRegionName == defaults.REGION_NAME_PLACEHOLDER) {
		cloudRegionName = TextInput("Enter a valid region name:")
	}

	log.Info("Got your project and region info, let's create the infra now...")
	content = ReplacePlaceholderValues(content, defaults.PROJECT_NAME_PLACEHOLDER, cloudProjectName)
	content = ReplacePlaceholderValues(content, defaults.REGION_NAME_PLACEHOLDER, cloudRegionName)
	content = ReplacePlaceholderValues(content, defaults.INSTANCE_NAME_PLACEHOLDER, cloudInstanceName)

	return content
}

// Delete an existing  Infra Definition
func deleteInfraDefinition(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Infra Definition
func listInfraDefinition(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}
