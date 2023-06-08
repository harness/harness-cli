package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create update  Infra Definition
func applyInfraDefinition(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create / update a Infrastructure Definition using the yaml.")
	createoOrUpdateInfraURL := GetUrlWithQueryParams("", "", "infrastructures", map[string]string{
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
	entityExists := getEntity(fmt.Sprintf("infrastructures/%s", identifier), projectIdentifier, orgIdentifier, map[string]string{
		"environmentIdentifier": environmentRef,
	})
	var resp ResponseBody
	var err error
	if !entityExists {
		println("Creating Infrastructure Definition with id: ", getColoredText(identifier, color.FgGreen))
		fmt.Println("createoOrUpdateInfraURL: ", createoOrUpdateInfraURL)
		fmt.Println("requestBody: ", requestBody)
		resp, err = Post(createoOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, JSON_CONTENT_TYPE)

		if err == nil {
			println(getColoredText("Infrastructure Definition created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found Infrastructure Definition with id: ", getColoredText(identifier, color.FgGreen))
		println(getColoredText("Updating infrastructure definition details....", color.FgGreen))
		resp, err = Put(createoOrUpdateInfraURL, cliCdRequestData.AuthToken, InfraPayload, JSON_CONTENT_TYPE)
		if err == nil {
			println(getColoredText("Infrastructure Definition updated successfully!", color.FgGreen))
			//printJson(resp.Data)
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
