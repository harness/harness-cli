package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// Create update Environment
func applyEnvironment(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create / update an environment using the yaml.")
	createOrUpdateEnvURL := GetUrlWithQueryParams("", NG_BASE_URL, ENVIRONMENT_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))

	requestBody := getJsonFromYaml(content)
	println("Request Body")
	printJson(requestBody)
	if requestBody == nil {
		println(getColoredText("Please enter valid environment yaml", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "environment", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "environment", "name").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "environment", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "environment", "orgIdentifier").(string))
	envType := valueToString(GetNestedValue(requestBody, "environment", "type").(string))
	//envColor := valueToString(GetNestedValue(requestBody, "environment", "color").(string))
	fmt.Printf("identifier=%s, name=%s",
		identifier, name)

	//setup payload for Environment create / update
	fmt.Printf("Before EnvPayload: ")
	EnvPayload := HarnessEnvironment{Identifier: identifier, Name: name, Type: envType,
		ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	fmt.Printf("EnvPayload: ", EnvPayload)

	entityExists := getEntity(NG_BASE_URL, fmt.Sprintf("%s/%s", ENVIRONMENT_ENDPOINT, identifier), projectIdentifier, orgIdentifier, map[string]string{})

	var resp ResponseBody
	var err error
	if !entityExists {
		println("Creating environment with id: ", getColoredText(identifier, color.FgGreen))
		fmt.Println("createOrUpdateEnvURL: ", createOrUpdateEnvURL)
		fmt.Println("requestBody: ", requestBody)
		resp, err = Post(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)

		if err == nil {
			println(getColoredText("Environment created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found Environment with id: ", getColoredText(identifier, color.FgGreen))
		println(getColoredText("Updating existing Environment Environment details....", color.FgGreen))
		resp, err = Put(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Environment updated successfully!", color.FgGreen))
			//printJson(resp.Data)
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
