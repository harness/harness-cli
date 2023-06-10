package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create or update Pipeline
func applyPipeline(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create / update pipeline using the yaml.")
	createOrUpdateEnvURL := GetUrlWithQueryParams("", PIPELINES_BASE_URL, PIPELINES_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))

	requestBody := getJsonFromYaml(content)
	println("Request Body")
	printJson(requestBody)
	if requestBody == nil {
		println(getColoredText("Please enter valid pipeline yaml file", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "pipeline", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "pipeline", "name").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "pipeline", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "pipeline", "orgIdentifier").(string))
	branch := valueToString(GetNestedValue(requestBody, "pipeline", "branch").(string))
	fmt.Printf("identifier=%s, name=%s",
		identifier, name)

	//setup payload for Pipelines create / update
	fmt.Printf("Before PipelinePayload: ")
	EnvPayload := HarnessPipeline{Identifier: identifier, Name: name, Branch: branch,
		ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	fmt.Printf("Pipeline Payload: ", EnvPayload)

	entityExists := getEntity(fmt.Sprintf("%/%s", PIPELINES_ENDPOINT, identifier), projectIdentifier, orgIdentifier, map[string]string{})

	var resp ResponseBody
	var err error
	if !entityExists {
		println("Creating pipeline with id: ", getColoredText(identifier, color.FgGreen))
		fmt.Println("createOrUpdateEnvURL: ", createOrUpdateEnvURL)
		fmt.Println("requestBody: ", requestBody)
		resp, err = Post(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)

		if err == nil {
			println(getColoredText("Pipeline created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found Pipeline with id: ", getColoredText(identifier, color.FgGreen))
		println(getColoredText("Updating existing Pipeline details....", color.FgGreen))
		resp, err = Put(createOrUpdateEnvURL, cliCdRequestData.AuthToken, EnvPayload, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Pipeline updated successfully!", color.FgGreen))
			//printJson(resp.Data)
			return nil
		}
	}

	return nil
}

// Delete an existing Pipeline
func deletePipeline(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Pipeline
func listPipeline(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
