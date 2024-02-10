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
        // TO-DO: print execution URL and remove magic strings
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
        pipelineEndpoint := fmt.Sprintf("%s/%s", defaults.PIPELINES_ENDPOINT, pipelineIdentifier)
        pipelineExists := GetEntity(baseURL, pipelineEndpoint, projectIdentifier, orgIdentifier, map[string]string{})
        if !pipelineExists {
                return fmt.Errorf("Could not fetch pipeline. Check pipeline id and scope.")
        }
        execPipelineEndpoint := GetPipelineExecUrl(c, pipelineIdentifier)       
        execPipelinePOSTEndpoint := GetUrlWithQueryParams("", execPipelineEndpoint, "", map[string]string{})
        resp, err := client.PipelineExecPost(execPipelinePOSTEndpoint, CliCdRequestData.AuthToken, nil, defaults.CONTENT_TYPE_JSON, nil)
        pipelineExecLink := GetPipelineExecLink(resp, CliCdRequestData.Account, orgIdentifier, projectIdentifier, pipelineIdentifier)
        if err == nil {
            println(GetColoredText("Successfully started execution of pipeline ", color.FgGreen) +
                   GetColoredText(pipelineIdentifier, color.FgBlue))
            println(GetColoredText("View execution at ", color.FgGreen) +
                   GetColoredText(pipelineExecLink , color.FgYellow))
            return nil
        }
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

func GetPipelineUrl(ctx *cli.Context, pipelineId string) string {
        projectPath := GetProjectUrl(ctx)
        pipelinesPath := fmt.Sprintf("%s%s/", projectPath, defaults.PIPELINES_ENDPOINT)
        fullPipelineUrl:= fmt.Sprintf("%s%s/", pipelinesPath, pipelineId)
        return fullPipelineUrl
}

func GetPipelineExecUrl(ctx *cli.Context, pipelineId string) string {
        pipelinePath := GetPipelineUrl(ctx, pipelineId)
        pipelineExecPath := fmt.Sprintf("%s%s", pipelinePath, defaults.EXECUTE_PIPELINE_ENDPOINT)
        return pipelineExecPath
}

func GetPipelineExecLink(respBodyObj PipelineExecRespBody, accountId string, orgId string, projectId string, pipelineId string) string {
        var baseUrl string
        if CliCdRequestData.BaseUrl == "" {
                baseUrl = defaults.HARNESS_PROD_URL
            } else {
                baseUrl = CliCdRequestData.BaseUrl
        }
        executionData := respBodyObj.ExecDetails.(map[string]interface{})
        executionId := executionData["execution_id"].(string)
        execLink := fmt.Sprintf("%s%s/%s/%s/%s%s%s/%s%s/%s/%s/%s/%s", baseUrl, "ng", 
            "account", accountId, "home/", 
            defaults.ORGANIZATIONS_ENDPOINT, orgId, 
            defaults.PROJECTS_ENDPOINT, projectId, 
            defaults.PIPELINES_ENDPOINT, pipelineId,
            "executions", executionId)  
       return execLink
}
