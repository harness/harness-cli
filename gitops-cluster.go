package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create or update a Gitops Cluster
func applyCluster(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getBaseUrl(c, GITOPS_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update cluster using the yaml=",
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
		println(getColoredText("Please enter valid cluster yaml file", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "gitops", "identifier").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "gitops", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "gitops", "orgIdentifier").(string))
	createOrUpdateClusterURL := GetUrlWithQueryParams("", baseURL, GITOPS_CLUSTER_ENDPOINT, map[string]string{
		"identifier":        identifier,
		"accountIdentifier": cliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	extraParams := map[string]string{
		"query.name": valueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
	}
	entityExists := getEntity(baseURL, fmt.Sprintf(GITOPS_CLUSTER_ENDPOINT+"/%s", identifier),
		projectIdentifier, orgIdentifier, extraParams)
	var _ ResponseBody
	var err error

	if !entityExists {
		println("Creating cluster with id: ", getColoredText(identifier, color.FgGreen))
		clusterPayload := createClusterPayload(requestBody)
		_, err = Post(createOrUpdateClusterURL, cliCdRequestData.AuthToken, clusterPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Successfully created cluster with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found GitOps Cluster with id=", getColoredText(identifier, color.FgCyan))
	}

	return nil
}

func createClusterPayload(requestBody map[string]interface{}) GitOpsCluster {
	newCluster := GitOpsCluster{
		Cluster: Cluster{
			Name:   valueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
			Server: valueToString(GetNestedValue(requestBody, "gitops", "cluster", "server").(string)),
			Config: Config{
				ClusterConnectionType: valueToString(GetNestedValue(requestBody, "gitops", "cluster", "config", "clusterConnectionType").(string)),
			},
		},
	}
	return newCluster
}
