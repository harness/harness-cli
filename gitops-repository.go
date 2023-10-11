package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"harness/client"
	"harness/defaults"
	"harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
)

var agentIdentifier = ""

// create or update a Gitops Repository
func applyRepository(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := GetBaseUrl(c, defaults.GITOPS_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update repository using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	var content, _ = ReadFromFile(c.String("file"))
	agentIdentifier = c.String("agent-identifier")
	if agentIdentifier == "" || agentIdentifier == defaults.GITOPS_AGENT_IDENTIFIER_PLACEHOLDER {
		agentIdentifier = TextInput("Enter a valid AgentIdentifier:")
	}
	content = ReplacePlaceholderValues(content, defaults.GITOPS_AGENT_IDENTIFIER_PLACEHOLDER, agentIdentifier)
	baseURL = baseURL + agentIdentifier
	requestBody := GetJsonFromYaml(content)
	if requestBody == nil {
		println(GetColoredText("Please enter valid repository yaml file", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "gitops", "identifier").(string))
	projectIdentifier := ValueToString(GetNestedValue(requestBody, "gitops", "projectIdentifier").(string))
	orgIdentifier := ValueToString(GetNestedValue(requestBody, "gitops", "orgIdentifier").(string))
	createOrUpdateRepositoryURL := GetUrlWithQueryParams("", baseURL, defaults.GITOPS_REPOSITORY_ENDPOINT, map[string]string{
		"identifier":        identifier,
		"accountIdentifier": shared.CliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	extraParams := map[string]string{
		"query.repo": ValueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
	}
	entityExists := GetEntity(baseURL, fmt.Sprintf(defaults.GITOPS_REPOSITORY_ENDPOINT+"/%s", identifier),
		projectIdentifier, orgIdentifier, extraParams)
	var _ ResponseBody
	var err error

	if !entityExists {
		println("Creating repository with id: ", GetColoredText(identifier, color.FgGreen))
		repoPayload := createRepoPayload(requestBody)
		_, err = client.Post(createOrUpdateRepositoryURL, shared.CliCdRequestData.AuthToken, repoPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created repository with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.REPO_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            GetTypeFromYAML(content),
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.REPO_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            GetTypeFromYAML(content),
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			return nil
		}
	} else {
		println("Found repository with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of repository with id=", GetColoredText(identifier, color.FgBlue))
		var repoPUTUrl = GetUrlWithQueryParams("", baseURL,
			fmt.Sprintf("%s/%s", defaults.GITOPS_REPOSITORY_ENDPOINT, identifier), map[string]string{
				"accountIdentifier": shared.CliCdRequestData.Account,
				"orgIdentifier":     orgIdentifier,
				"projectIdentifier": projectIdentifier,
			})
		newRepoPayload := createRepoPUTPayload(requestBody)
		_, err = client.Put(repoPUTUrl, shared.CliCdRequestData.AuthToken, newRepoPayload, defaults.CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(GetColoredText("Successfully updated repository with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.REPO_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            GetTypeFromYAML(content),
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			return nil
		}
	}

	return nil
}

func createRepoPayload(requestBody map[string]interface{}) GitOpsRepository {
	newRepo := GitOpsRepository{Repo: Repo{
		Name:           ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
		ConnectionType: ValueToString(GetNestedValue(requestBody, "gitops", "repo", "connectionType").(string)),
		Repo:           ValueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
		Type:           ValueToString(GetNestedValue(requestBody, "gitops", "repo", "type").(string))}}
	return newRepo
}

func createRepoPUTPayload(requestBody map[string]interface{}) RepoWithUpdateMask {
	repoWithUpdateMask := RepoWithUpdateMask{
		Repo: Repo{
			Name:           ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
			ConnectionType: ValueToString(GetNestedValue(requestBody, "gitops", "repo", "connectionType").(string)),
			Repo:           ValueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
			Type:           ValueToString(GetNestedValue(requestBody, "gitops", "repo", "type").(string))},
		UpdateMask: struct {
			Paths []string `json:"paths"`
		}(UpdateMask{Paths: []string{"name", "connectionType", "authType"}})}

	return repoWithUpdateMask
}
