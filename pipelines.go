package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create or update Pipeline
func applyPipeline(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create or update pipeline using the given yaml.")

	var content = readFromFile(c.String("file"))
	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid pipeline yaml file", color.FgRed))
	}

	identifier := valueToString(GetNestedValue(requestBody, "pipeline", "identifier").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "pipeline", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "pipeline", "orgIdentifier").(string))
	createOrUpdatePipelineURL := GetUrlWithQueryParams("", PIPELINES_BASE_URL, PIPELINES_ENDPOINT_V2, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	entityExists := getEntity(PIPELINES_BASE_URL, fmt.Sprintf("%s/%s", PIPELINES_ENDPOINT, identifier),
		projectIdentifier, orgIdentifier, map[string]string{})
	var _ ResponseBody
	var err error
	if !entityExists {
		println("Creating pipeline with id: ", getColoredText(identifier, color.FgCyan))
		fmt.Println("createOrUpdatePipelineURL: ", createOrUpdatePipelineURL)
		fmt.Println("requestBody: ", requestBody)
		_, err = Post(createOrUpdatePipelineURL, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_YAML)

		if err == nil {
			println(getColoredText("Pipeline created successfully!", color.FgGreen))
			return nil
		}
	} else {
		var pipelinesPUTUrl = GetUrlWithQueryParams("", PIPELINES_BASE_URL,
			fmt.Sprintf("%s/%s", PIPELINES_ENDPOINT_V2, identifier), map[string]string{
				"pipelineIdentifier": identifier,
				"accountIdentifier":  cliCdRequestData.Account,
				"orgIdentifier":      orgIdentifier,
				"projectIdentifier":  projectIdentifier,
			})
		println("Found Pipeline with id: ", getColoredText(identifier, color.FgBlue))
		println(getColoredText("Updating existing Pipeline details....", color.FgGreen))
		_, err = Put(pipelinesPUTUrl, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_YAML)
		if err == nil {
			println(getColoredText("Pipeline updated successfully!", color.FgGreen))
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
