package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	"harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

// create or update a Gitops Cluster
func applyCluster(c *cli.Context) error {
	filePath := c.String("file")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")

	baseURL := GetBaseUrl(c, defaults.GITOPS_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update cluster using the yaml=",
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
		println(GetColoredText("Please enter valid cluster yaml file", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "gitops", "identifier").(string))

	if projectIdentifier == "" {
		projectIdentifier = ValueToString(GetNestedValue(requestBody, "gitops", "projectIdentifier").(string))
	}

	if orgIdentifier == "" {
		orgIdentifier = ValueToString(GetNestedValue(requestBody, "gitops", "orgIdentifier").(string))
	}

	createOrUpdateClusterURL := GetUrlWithQueryParams("", baseURL, defaults.GITOPS_CLUSTER_ENDPOINT, map[string]string{
		"identifier":        identifier,
		"accountIdentifier": shared.CliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
	})
	extraParams := map[string]string{
		"query.name": ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
	}
	entityExists := GetEntity(baseURL, fmt.Sprintf(defaults.GITOPS_CLUSTER_ENDPOINT+"/%s", identifier),
		projectIdentifier, orgIdentifier, extraParams)
	var _ ResponseBody
	var err error

	if !entityExists {
		println("Creating cluster with id: ", GetColoredText(identifier, color.FgGreen))
		clusterPayload := createClusterPayload(requestBody)
		_, err = client.Post(createOrUpdateClusterURL, shared.CliCdRequestData.AuthToken, clusterPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created cluster with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.CLUSTER_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": shared.CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    shared.CliCdRequestData.UserId,
			})
			return nil
		}
	} else {
		println("Found GitOps Cluster with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of GitOps Cluster with id=", GetColoredText(identifier, color.FgBlue))
		var clusterPUTUrl = GetUrlWithQueryParams("", baseURL,
			fmt.Sprintf("%s/%s", defaults.GITOPS_CLUSTER_ENDPOINT, identifier), map[string]string{
				"accountIdentifier": shared.CliCdRequestData.Account,
				"orgIdentifier":     orgIdentifier,
				"projectIdentifier": projectIdentifier,
				"agentIdentifier":   agentIdentifier,
			})
		newClusterPayload := createClusterPUTPayload(requestBody)
		_, err = client.Put(clusterPUTUrl, shared.CliCdRequestData.AuthToken, newClusterPayload, defaults.CONTENT_TYPE_JSON, nil)

		if err == nil {
			println(GetColoredText("Successfully updated GitOps Cluster with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.CLUSTER_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
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

func createClusterPayload(requestBody map[string]interface{}) GitOpsCluster {
	newCluster := GitOpsCluster{
		Cluster: Cluster{
			Name:   ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
			Server: ValueToString(GetNestedValue(requestBody, "gitops", "cluster", "server").(string)),
			Config: Config{
				ClusterConnectionType: ValueToString(GetNestedValue(requestBody, "gitops", "cluster", "config", "clusterConnectionType").(string)),
			},
		},
	}
	return newCluster
}

func createClusterPUTPayload(requestBody map[string]interface{}) GitOpsClusterWithUpdatedFields {
	clusterWithUpdateMask := GitOpsClusterWithUpdatedFields{
		Cluster: Cluster{
			Name:   ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
			Server: ValueToString(GetNestedValue(requestBody, "gitops", "cluster", "server").(string)),
			Config: Config{
				ClusterConnectionType: ValueToString(GetNestedValue(requestBody, "gitops", "cluster", "config", "clusterConnectionType").(string)),
			},
		},
		UpdatedFields: []string{"name", "tags", "authType", "credsType"},
	}
	return clusterWithUpdateMask
}
