package main

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var gcpProjectName = ""
var gcpRegionName = ""

// create or update  Infra Definition
func applyInfraDefinition(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	gcpProjectName = c.String("gcp-project")
	gcpRegionName = c.String("region")
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
	var infraType = fetchCloudType(content)
	switch {
	case infraType == GCP:
		log.Info("Looks like you are creating an infrastructure definition for GCP," +
			" validating GCP project and region now...")
		if gcpProjectName == "" || gcpProjectName == GCP_PROJECT_NAME_PLACEHOLDER {
			gcpProjectName = TextInput("Enter a valid GCP project name:")
		}

		if gcpRegionName == "" || gcpRegionName == GCP_REGION_NAME_PLACEHOLDER {
			gcpRegionName = TextInput("Enter a valid GCP region name:")
		}
		log.Info("Got your gcp project and region info, let's create the infra now...")
		content = replacePlaceholderValues(content, GCP_PROJECT_NAME_PLACEHOLDER, gcpProjectName)
		content = replacePlaceholderValues(content, GCP_REGION_NAME_PLACEHOLDER, gcpRegionName)
	case infraType == AWS:
		log.Info("Looks like you are creating an infrastructure definition for AWS, validating yaml now...")
	default:
		return content
	}

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
