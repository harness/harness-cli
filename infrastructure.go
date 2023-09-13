package main

import (
	"fmt"
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
	baseURL := getNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	cloudProjectName = c.String("cloud-project")
	cloudRegionName = c.String("cloud-region")
	cloudInstanceName = c.String("instance-name")

	fmt.Println("Trying to create or update infrastructure using the given yaml=",
		getColoredText(filePath, color.FgCyan))

	createOrUpdateInfraURL := GetUrlWithQueryParams("", baseURL, INFRA_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content, _ = readFromFile(c.String("file"))
	content = updateInfraYamlContent(content)

	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter a valid yaml and try again...", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "infrastructureDefinition", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "infrastructureDefinition", "name").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "infrastructureDefinition", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "infrastructureDefinition", "orgIdentifier").(string))
	environmentRef := valueToString(GetNestedValue(requestBody, "infrastructureDefinition", "environmentRef").(string))
	//setup payload for Infra create / update
	InfraPayload := HarnessInfra{Identifier: identifier, Name: name, ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := getEntity(baseURL, fmt.Sprintf("infrastructures/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{
			"environmentIdentifier": environmentRef,
		})
	var err error
	if !entityExists {
		println("Creating infrastructure with id: ", getColoredText(identifier, color.FgGreen))
		_, err = Post(createOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Infrastructure Definition created successfully!", color.FgGreen))
			return nil
		}
	} else {
		println("Found infrastructure with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of infrastructure with id=", getColoredText(identifier, color.FgBlue))
		_, err = Put(createOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Successfully updated infrastructure definition with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}

	return nil
}

func updateInfraYamlContent(content string) string {
	hasRegionName := strings.Contains(content, REGION_NAME_PLACEHOLDER)
	hasProjectName := strings.Contains(content, PROJECT_NAME_PLACEHOLDER)
	hasInstanceName := strings.Contains(content, INSTANCE_NAME_PLACEHOLDER)

	if hasProjectName && (cloudProjectName == "" || cloudProjectName == PROJECT_NAME_PLACEHOLDER) {
		cloudProjectName = TextInput("Enter a valid project name:")
	}

	if hasInstanceName && (cloudInstanceName == "" || cloudInstanceName == INSTANCE_NAME_PLACEHOLDER) {
		cloudInstanceName = TextInput("Enter a valid instance name:")
	}

	if hasRegionName && (cloudRegionName == "" || cloudRegionName == REGION_NAME_PLACEHOLDER) {
		cloudRegionName = TextInput("Enter a valid region name:")
	}

	log.Info("Got your project and region info, let's create the infra now...")
	content = replacePlaceholderValues(content, PROJECT_NAME_PLACEHOLDER, cloudProjectName)
	content = replacePlaceholderValues(content, REGION_NAME_PLACEHOLDER, cloudRegionName)
	content = replacePlaceholderValues(content, INSTANCE_NAME_PLACEHOLDER, cloudInstanceName)

	return content
}

// Delete an existing  Infra Definition
func deleteInfraDefinition(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Infra Definition
func listInfraDefinition(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
