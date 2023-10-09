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

// create or update Pipeline
func applyPipeline(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := GetBaseUrl(c, defaults.PIPELINES_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update pipeline using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	var content, _ = ReadFromFile(c.String("file"))

	requestBody := GetJsonFromYaml(content)

	if requestBody == nil {
		println(GetColoredText("Please enter valid pipeline yaml file", color.FgRed))
	}

	identifier := ValueToString(GetNestedValue(requestBody, "pipeline", "identifier").(string))
	projectIdentifier := ValueToString(GetNestedValue(requestBody, "pipeline", "projectIdentifier").(string))
	orgIdentifier := ValueToString(GetNestedValue(requestBody, "pipeline", "orgIdentifier").(string))
	createOrUpdatePipelineURL := GetUrlWithQueryParams("", baseURL, defaults.PIPELINES_ENDPOINT_V2, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	entityExists := GetEntity(baseURL, fmt.Sprintf("%s/%s", defaults.PIPELINES_ENDPOINT, identifier),
		projectIdentifier, orgIdentifier, map[string]string{})
	var _ ResponseBody
	var err error
	if !entityExists {
		println("Creating pipeline with id: ", GetColoredText(identifier, color.FgGreen))
		_, err = client.Post(createOrUpdatePipelineURL, CliCdRequestData.AuthToken, requestBody, defaults.CONTENT_TYPE_YAML, nil)
		if err == nil {
			println(GetColoredText("Successfully created pipeline with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.PIPELINE_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":          CliCdRequestData.Account,
				"type":               GetTypeFromYAML(content),
				"userId":             CliCdRequestData.UserId,
				"pipelineIdentifier": identifier,
			})
			return nil
		}
	} else {
		var pipelinesPUTUrl = GetUrlWithQueryParams("", baseURL,
			fmt.Sprintf("%s/%s", defaults.PIPELINES_ENDPOINT_V2, identifier), map[string]string{
				"pipelineIdentifier": identifier,
				"accountIdentifier":  CliCdRequestData.Account,
				"orgIdentifier":      orgIdentifier,
				"projectIdentifier":  projectIdentifier,
			})
		println("Found pipeline with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of pipeline with id=", GetColoredText(identifier, color.FgBlue))
		_, err = client.Put(pipelinesPUTUrl, CliCdRequestData.AuthToken, requestBody, defaults.CONTENT_TYPE_YAML, nil)
		if err == nil {
			println(GetColoredText("Successfully updated pipeline with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.PIPELINE_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":          CliCdRequestData.Account,
				"type":               GetTypeFromYAML(content),
				"userId":             CliCdRequestData.UserId,
				"pipelineIdentifier": identifier,
			})
			return nil
		}
	}

	return nil
}

// Delete an existing Pipeline
func deletePipeline(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}

// Delete an existing Pipeline
func listPipeline(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}
