package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
	"regexp"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create or update Pipeline
func applyPipeline(c *cli.Context) error {
	filePath := c.String("file")
	dockerUsername := c.String("docker-user")
	githubUsername := c.String("git-user")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")

	baseURL := GetBaseUrl(c, defaults.PIPELINES_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update pipeline using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	var content, _ = ReadFromFile(c.String("file"))

	if yamlHasDockerUsername(content) {
		if dockerUsername == "" || dockerUsername == defaults.DOCKER_USERNAME_PLACEHOLDER {
			dockerUsername = TextInput("Enter a valid docker username:")
		}
		content = ReplacePlaceholderValues(content, defaults.DOCKER_USERNAME_PLACEHOLDER, dockerUsername)
	}
	if yamlHasGithubUsername(content) {
		if githubUsername == "" || githubUsername == defaults.GITHUB_USERNAME_PLACEHOLDER {
			githubUsername = TextInput("Enter valid github username:")
		}
		content = ReplacePlaceholderValues(content, defaults.GITHUB_USERNAME_PLACEHOLDER, githubUsername)
	}
	requestBody := GetJsonFromYaml(content)

	if requestBody == nil {
		println(GetColoredText("Please enter valid pipeline yaml file", color.FgRed))
	}

	identifier := ValueToString(GetNestedValue(requestBody, "pipeline", "identifier").(string))
	if orgIdentifier != "" {
		content = ReplacePlaceholderValues(content, defaults.DEFAULT_ORG, orgIdentifier)
	} else {
		orgIdentifier = ValueToString(GetNestedValue(requestBody, "pipeline", "orgIdentifier").(string))
	}
	if projectIdentifier != "" {
		content = ReplacePlaceholderValues(content, defaults.DEFAULT_PROJECT, projectIdentifier)
	} else {
		projectIdentifier = ValueToString(GetNestedValue(requestBody, "pipeline", "projectIdentifier").(string))
	}

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
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.PIPELINE_UPDATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
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

// List details of an existing Pipeline
func listPipeline(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
        return nil
}

// Run an existing Pipeline
func runPipeline(c *cli.Context) error {
        // Get path and query parameters from user and/or defaults
        baseURL := GetBaseUrl(c, defaults.PIPELINES_BASE_URL)	
        orgIdentifier := c.String("org-id")
        if orgIdentifier == "" {
                orgIdentifier = defaults.DEFAULT_ORG
        }
        projectIdentifier := c.String("project-id")
        if projectIdentifier == "" {
                projectIdentifier = defaults.DEFAULT_PROJECT
        }
        pipelineIdentifier := c.String ("pipeline-id")
        if pipelineIdentifier == "" {
                return fmt.Errorf("Pipeline id required. See: harness pipeline run --help") 
        }

        // Process optional inputs YAML into request body
        var content, _ = ReadFromFile(c.String("inputs-file"))
        requestBody := GetJsonFromYaml(content)

        // Return error if pipeline doesn't exist
        pipelineEndpoint := fmt.Sprintf("%s/%s", defaults.PIPELINES_ENDPOINT, pipelineIdentifier)
        pipelineExists := GetEntity(baseURL, pipelineEndpoint, projectIdentifier, orgIdentifier, map[string]string{})
        if !pipelineExists {
                return fmt.Errorf("Could not fetch pipeline. Check pipeline id and scope.")
        }

        // Generate endpoint, tack on query parameters
        execPipelineEndpoint := fmt.Sprintf("%s/%s", defaults.EXECUTE_PIPELINE_ENDPOINT, pipelineIdentifier)     
        execPipelineURL := GetUrlWithQueryParams("", baseURL, execPipelineEndpoint, map[string]string{
                "accountIdentifier": CliCdRequestData.Account,
                "orgIdentifier":     orgIdentifier,
                "projectIdentifier": projectIdentifier,
        })

        // Make execute pipeline POST call
        var err error
        var resp ResponseBody
        resp, err = client.Post(execPipelineURL, CliCdRequestData.AuthToken, requestBody, defaults.CONTENT_TYPE_YAML, nil)
        
        // Break and write to console if error 
        if err != nil {
            return fmt.Errorf("Could not start execution of pipeline %s. Check pipeline configuration and inputs.", pipelineIdentifier)
        }

        // Otherwise print success message and execution link
        pipelineExecLink := GetPipelineExecLink(resp, CliCdRequestData.Account, orgIdentifier, projectIdentifier, pipelineIdentifier)
        println(GetColoredText("Successfully started execution of pipeline ", color.FgGreen) +
               GetColoredText(pipelineIdentifier, color.FgBlue))
        println(GetColoredText("Execution URL: ", color.FgGreen) +
               GetColoredText(pipelineExecLink , color.FgYellow))
        return nil
}

func yamlHasDockerUsername(str string) bool {
	regexPattern := `value:\s+DOCKER_USERNAME`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

func yamlHasGithubUsername(str string) bool {
	regexPattern := `github.com/GITHUB_USERNAME`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

func GetPipelineExecLink(respBodyObj ResponseBody, accountId string, orgId string, projectId string, pipelineId string) string {
        // Get Harness host from user or use default
        var baseUrl string
        if CliCdRequestData.BaseUrl == "" {
                baseUrl = defaults.HARNESS_PROD_URL
            } else {
                baseUrl = CliCdRequestData.BaseUrl
        }

        // Get pipeline execution ID from custom HTTP reponse object
        execData := respBodyObj.Data.(map[string]interface{})
        planExecutionData := execData["planExecution"].(map[string]interface{})
        execId := planExecutionData["uuid"]
        // Build pipeline execution URL
        // Start with individual entity segments
        uxSegement := fmt.Sprintf("%s%s/", baseUrl, defaults.HARNESS_UX_VERSION)
        accountSegment := fmt.Sprintf("%s/%s/%s/", "account", accountId, "home")
        orgSegment := fmt.Sprintf("%s%s/", defaults.ORGANIZATIONS_ENDPOINT, orgId)
        projectSegment := fmt.Sprintf("%s%s/", defaults.PROJECTS_ENDPOINT, projectId)
        pipelineSegment := fmt.Sprintf("%s/%s/", defaults.PIPELINES_ENDPOINT, pipelineId) 
        execSegment := fmt.Sprintf("%s/%s/", "executions", execId) 
 
        // From previous segments, build user's project path and pipeline execution path
        uxProjectPath := fmt.Sprintf("%s%s%s%s", uxSegement, accountSegment, orgSegment, projectSegment)
        pipelineExecPath := fmt.Sprintf("%s%s", pipelineSegment, execSegment)

        // Cat the ux and pipieline exec paths into complete URL
        fullPipelineExecLink := fmt.Sprintf("%s%s", uxProjectPath, pipelineExecPath)
        return fullPipelineExecLink
} 
