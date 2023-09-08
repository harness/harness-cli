package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var agentIdentifier = ""

// create or update a Gitops Repository
func applyRepository(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getBaseUrl(c, GITOPS_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update repository using the yaml=",
		getColoredText(filePath, color.FgCyan))
	var content, _ = readFromFile(c.String("file"))
	agentIdentifier = c.String("agent-identifier")
	if agentIdentifier == "" || agentIdentifier == GITOPS_AGENT_IDENTIFIER_PLACEHOLDER {
		agentIdentifier = TextInput("Enter a valid AgentIdentifier:")
	}
	content = replacePlaceholderValues(content, GITOPS_AGENT_IDENTIFIER_PLACEHOLDER, agentIdentifier)
	baseURL = baseURL + agentIdentifier
	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid repository yaml file", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "gitops", "identifier").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "gitops", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "gitops", "orgIdentifier").(string))
	createOrUpdateRepositoryURL := GetUrlWithQueryParams("", baseURL, GITOPS_REPOSITORY_ENDPOINT, map[string]string{
		"identifier":        identifier,
		"accountIdentifier": cliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	extraParams := map[string]string{
		"query.repo": valueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
	}
	entityExists := getEntity(baseURL, fmt.Sprintf(GITOPS_REPOSITORY_ENDPOINT+"/%s", identifier),
		projectIdentifier, orgIdentifier, extraParams)
	var _ ResponseBody
	var err error

	if !entityExists {
		println("Creating repository with id: ", getColoredText(identifier, color.FgGreen))
		repoPayload := createRepoPayload(requestBody)
		_, err = Post(createOrUpdateRepositoryURL, cliCdRequestData.AuthToken, repoPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Successfully created repository with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found repository with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of repository with id=", getColoredText(identifier, color.FgBlue))
		var repoPUTUrl = GetUrlWithQueryParams("", baseURL,
			fmt.Sprintf("%s/%s", GITOPS_REPOSITORY_ENDPOINT, identifier), map[string]string{
				"accountIdentifier": cliCdRequestData.Account,
				"orgIdentifier":     orgIdentifier,
				"projectIdentifier": projectIdentifier,
			})
		newRepoPayload := createRepoPUTPayload(requestBody)
		_, err = Put(repoPUTUrl, cliCdRequestData.AuthToken, newRepoPayload, CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(getColoredText("Successfully updated repository with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}

	return nil
}

func createRepoPayload(requestBody map[string]interface{}) GitOpsRepository {
	newRepo := GitOpsRepository{Repo: Repo{
		Name:           valueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
		ConnectionType: valueToString(GetNestedValue(requestBody, "gitops", "repo", "connectionType").(string)),
		Repo:           valueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
		Type:           valueToString(GetNestedValue(requestBody, "gitops", "repo", "type").(string))}}
	return newRepo
}

func createRepoPUTPayload(requestBody map[string]interface{}) RepoWithUpdateMask {
	repoWithUpdateMask := RepoWithUpdateMask{
		Repo: Repo{
			Name:           valueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
			ConnectionType: valueToString(GetNestedValue(requestBody, "gitops", "repo", "connectionType").(string)),
			Repo:           valueToString(GetNestedValue(requestBody, "gitops", "repo", "repo").(string)),
			Type:           valueToString(GetNestedValue(requestBody, "gitops", "repo", "type").(string))},
		UpdateMask: struct {
			Paths []string `json:"paths"`
		}(UpdateMask{Paths: []string{"name", "connectionType", "authType"}})}

	return repoWithUpdateMask
}
