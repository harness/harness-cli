package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	"harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
	"regexp"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applyGitopsApplications(c *cli.Context) error {
	filePath := c.String("file")
	githubUsername := c.String("git-user")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")

	baseURL := GetBaseUrl(c, defaults.GITOPS_BASE_URL)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update gitops-application using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	var content, _ = ReadFromFile(c.String("file"))
	if isGithubRepoUrl(content) {
		if githubUsername == "" || githubUsername == defaults.GITHUB_USERNAME_PLACEHOLDER {
			githubUsername = TextInput("Enter valid github username:")
		}
		content = ReplacePlaceholderValues(content, defaults.GITHUB_USERNAME_PLACEHOLDER, githubUsername)
	}

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
	if orgIdentifier == "" {
		orgIdentifier = ValueToString(GetNestedValue(requestBody, "gitops", "orgIdentifier").(string))
	}
	if projectIdentifier == "" {
		projectIdentifier = ValueToString(GetNestedValue(requestBody, "gitops", "projectIdentifier").(string))
	}
	clusterIdentifier := ValueToString(GetNestedValue(requestBody, "gitops", "clusterIdentifier").(string))
	repoIdentifier := ValueToString(GetNestedValue(requestBody, "gitops", "repoIdentifier").(string))

	createOrUpdateApplicationURL := GetUrlWithQueryParams("", baseURL, defaults.GITOPS_APPLICATION_ENDPOINT, map[string]string{
		"accountIdentifier": shared.CliCdRequestData.Account,
		"orgIdentifier":     orgIdentifier,
		"projectIdentifier": projectIdentifier,
		"clusterIdentifier": clusterIdentifier,
		"repoIdentifier":    repoIdentifier,
	})

	applicationName := ValueToString(GetNestedValue(requestBody, "gitops", "name").(string))
	syncApplicationURL := GetUrlWithQueryParams("", baseURL,
		fmt.Sprintf(defaults.GITOPS_APPLICATION_ENDPOINT+"/%s", applicationName+"/sync"), map[string]string{
			"routingId":         shared.CliCdRequestData.Account,
			"accountIdentifier": shared.CliCdRequestData.Account,
			"orgIdentifier":     orgIdentifier,
			"projectIdentifier": projectIdentifier,
		})

	extraParams := map[string]string{
		"agentIdentifier": agentIdentifier,
	}
	entityExists := GetEntity(baseURL, fmt.Sprintf(defaults.GITOPS_APPLICATION_ENDPOINT+"/%s", applicationName),
		projectIdentifier, orgIdentifier, extraParams)
	var _ ResponseBody
	var err error

	if !entityExists {
		println("Creating GitOps-Application with id: ", GetColoredText(applicationName, color.FgGreen))
		applicationPayload := createGitOpsApplicationPayload(requestBody)
		_, err = client.Post(createOrUpdateApplicationURL, shared.CliCdRequestData.AuthToken, applicationPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created GitOps-Application with id= ", color.FgGreen) +
				GetColoredText(applicationName, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.APP_CREATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":       shared.CliCdRequestData.Account,
				"type":            GetTypeFromYAML(content),
				"userId":          shared.CliCdRequestData.UserId,
				"agentIdentifier": agentIdentifier,
			})
			return nil
		}
	} else {
		println("Found GitOps Application with id=", GetColoredText(applicationName, color.FgCyan))
		println("Updating details of GitOps Application with id=", GetColoredText(applicationName, color.FgBlue))

		var appPUTUrl = GetUrlWithQueryParams("", baseURL,
			fmt.Sprintf(defaults.GITOPS_APPLICATION_ENDPOINT+"/%s", applicationName), map[string]string{
				"routingId":         shared.CliCdRequestData.Account,
				"accountIdentifier": shared.CliCdRequestData.Account,
				"orgIdentifier":     orgIdentifier,
				"projectIdentifier": projectIdentifier,
				"repoIdentifier":    repoIdentifier,
				"clusterIdentifier": clusterIdentifier,
			})
		newAppPayload := createGitOpsApplicationPUTPayload(requestBody)
		syncPayload := createGitOpsApplicationPayload(requestBody)
		println("Syncing the GitOps Application before updating the spec:", GetColoredText(applicationName, color.FgGreen))
		_, err = client.Post(syncApplicationURL, shared.CliCdRequestData.AuthToken, syncPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully synced GitOps app with id= ", color.FgGreen) +
				GetColoredText(applicationName, color.FgBlue))
			_, err = client.Put(appPUTUrl, shared.CliCdRequestData.AuthToken, newAppPayload, defaults.CONTENT_TYPE_JSON, nil)
			if err == nil {
				println(GetColoredText("Successfully updated GitOps app with id= ", color.FgGreen) +
					GetColoredText(applicationName, color.FgBlue))
				telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.APP_UPDATED, UserId: shared.CliCdRequestData.UserId}, map[string]interface{}{
					"accountId":       shared.CliCdRequestData.Account,
					"type":            GetTypeFromYAML(content),
					"userId":          shared.CliCdRequestData.UserId,
					"agentIdentifier": agentIdentifier,
				})
				return nil

			}
			return nil
		}
	}

	return nil
}

func createGitOpsApplicationPayload(requestBody map[string]interface{}) GitOpsApplication {
	newApplication := GitOpsApplication{
		Application: Application{
			Metadata: Metadata{
				Labels: Labels{
					Envref:     ValueToString(GetNestedValue(requestBody, "gitops", "application", "metadata", "labels", "harness.io/envRef").(string)),
					Serviceref: ValueToString(GetNestedValue(requestBody, "gitops", "application", "metadata", "labels", "harness.io/serviceRef").(string)),
				},
				Name:        ValueToString(GetNestedValue(requestBody, "gitops", "name").(string)),
				ClusterName: ValueToString(GetNestedValue(requestBody, "gitops", "application", "metadata", "clusterName").(string)),
			},
			Spec: Spec{
				Source: Source{
					RepoURL:        ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "repoURL").(string)),
					Path:           ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "path").(string)),
					TargetRevision: ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "targetRevision").(string)),
				},
				Destination: Destination{
					Server:    ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "destination", "server").(string)),
					Namespace: ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "destination", "namespace").(string)),
				},
			},
		},
	}
	return newApplication
}

func createGitOpsApplicationPUTPayload(requestBody map[string]interface{}) GitOpsApplication {
	putApp := GitOpsApplication{
		Application{
			Metadata: Metadata{
				Labels: Labels{
					Serviceref: ValueToString(GetNestedValue(requestBody, "gitops", "application", "metadata", "labels", "harness.io/serviceRef").(string)),
					Envref:     ValueToString(GetNestedValue(requestBody, "gitops", "application", "metadata", "labels", "harness.io/envRef").(string)),
				},
			},
			Spec: Spec{
				Source: Source{
					RepoURL:        ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "repoURL").(string)),
					Path:           ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "path").(string)),
					TargetRevision: ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "source", "targetRevision").(string)),
				},
				Destination: Destination{
					Server:    ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "destination", "server").(string)),
					Namespace: ValueToString(GetNestedValue(requestBody, "gitops", "application", "spec", "destination", "namespace").(string)),
				},
			},
		},
	}
	return putApp
}

func isGithubRepoUrl(str string) bool {
	regexPattern := `repoURL:\s+https:\/\/github.com\/GITHUB_USERNAME`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}
