package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create update  Infra Definition
func applyInfraDefinition(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update infrastructure using the yaml=",
		getColoredText(filePath, color.FgCyan))

	createOrUpdateInfraURL := GetUrlWithQueryParams("", baseURL, INFRA_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))
	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid yaml", color.FgRed))
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
		_, err = Post(createOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Infrastructure Definition created successfully!", color.FgGreen))
			return nil
		}
	} else {
		println("Found infrastructure with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of infrastructure with id=", getColoredText(identifier, color.FgBlue))
		_, err = Put(createOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully updated connector with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}

	return nil
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
