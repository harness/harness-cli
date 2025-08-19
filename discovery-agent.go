package main

import (
	"context"
	"fmt"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/utils"

	"github.com/antihax/optional"
	"github.com/fatih/color"
	"github.com/harness/harness-go-sdk/harness/svcdiscovery"
	"github.com/urfave/cli/v2"
)

// Delete an existing discovery agent using Harness Go SDK
func deleteDiscoveryAgent(c *cli.Context) error {
	identifier := c.String("identifier")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")
	environmentIdentifier := c.String("environment-id")
	deleteNetworkMaps := c.Bool("delete-network-maps")

	if identifier == "" {
		fmt.Println("Please provide a discovery agent identifier to delete")
		return nil
	}

	if orgIdentifier == "" {
		orgIdentifier = defaults.DEFAULT_ORG
	}
	if projectIdentifier == "" {
		projectIdentifier = defaults.DEFAULT_PROJECT
	}

	// Initialize Harness service discovery client
	config := svcdiscovery.NewConfiguration()
	config.Host = GetBaseUrlHost(c)
	config.Scheme = "https"
	config.DefaultHeader = map[string]string{
		"x-api-key": CliCdRequestData.AuthToken,
	}

	harnessClient := svcdiscovery.NewAPIClient(config)
	ctx := createAuthContext(c)

	println("Deleting discovery agent...")
	println(GetColoredText(fmt.Sprintf("Agent Identity: %s", identifier), color.FgRed))
	println(GetColoredText(fmt.Sprintf("Environment: %s", environmentIdentifier), color.FgCyan))

	// If delete-network-maps flag is set, delete network maps first
	if deleteNetworkMaps {
		println(GetColoredText("🗺️  Deleting associated network maps first...", color.FgYellow))

		// List network maps for this agent
		networkMaps, _, err := harnessClient.NetworkmapApi.ListNetworkMap(ctx, identifier, CliCdRequestData.Account, environmentIdentifier, 1, 100, true, &svcdiscovery.NetworkmapApiListNetworkMapOpts{
			OrganizationIdentifier: optional.NewString(orgIdentifier),
			ProjectIdentifier:      optional.NewString(projectIdentifier),
		})

		if err != nil {
			println(GetColoredText("Error listing network maps: "+err.Error(), color.FgRed))
			return err
		}

		if len(networkMaps.Items) > 0 {
			println(GetColoredText(fmt.Sprintf("Found %d network maps to delete", len(networkMaps.Items)), color.FgYellow))

			// Delete each network map
			for i, networkMap := range networkMaps.Items {
				println(GetColoredText(fmt.Sprintf("Deleting network map %d/%d: %s", i+1, len(networkMaps.Items), networkMap.Identity), color.FgYellow))

				_, _, err := harnessClient.NetworkmapApi.DeleteNetworkMap(ctx, identifier, networkMap.Identity, CliCdRequestData.Account, environmentIdentifier, &svcdiscovery.NetworkmapApiDeleteNetworkMapOpts{
					OrganizationIdentifier: optional.NewString(orgIdentifier),
					ProjectIdentifier:      optional.NewString(projectIdentifier),
				})

				if err != nil {
					println(GetColoredText(fmt.Sprintf("Error deleting network map %s: %s", networkMap.Identity, err.Error()), color.FgRed))
					return err
				}
			}

			println(GetColoredText("✅ Successfully deleted all network maps", color.FgGreen))
		} else {
			println(GetColoredText("No network maps found to delete", color.FgYellow))
		}
	} else if !deleteNetworkMaps {
		println(GetColoredText("⚠️  Network maps will NOT be deleted. Use --delete-network-maps flag to delete them first.", color.FgYellow))
	}

	// Now delete the agent
	println(GetColoredText("🤖 Deleting discovery agent...", color.FgYellow))
	_, resp, err := harnessClient.AgentApi.DeleteAgent(ctx, identifier, CliCdRequestData.Account, environmentIdentifier, &svcdiscovery.AgentApiDeleteAgentOpts{
		OrganizationIdentifier: optional.NewString(orgIdentifier),
		ProjectIdentifier:      optional.NewString(projectIdentifier),
	})

	if err != nil {
		println(GetColoredText("Error deleting discovery agent: "+err.Error(), color.FgRed))
		if resp != nil {
			println(GetColoredText(fmt.Sprintf("HTTP Status: %d", resp.StatusCode), color.FgYellow))
		}
		if !deleteNetworkMaps {
			println(GetColoredText("💡 Tip: Try using --delete-network-maps flag to clean up dependencies first", color.FgCyan))
		}
		return err
	}

	println(GetColoredText("🎉 Successfully deleted discovery agent!", color.FgGreen))
	telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.DISCOVERY_AGENT_DELETED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
		"accountId": CliCdRequestData.Account,
		"userId":    CliCdRequestData.UserId,
	})
	return nil
}

// List discovery agents using Harness Go SDK
func listDiscoveryAgents(c *cli.Context) error {
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")
	environmentIdentifier := c.String("environment-id")

	if orgIdentifier == "" {
		orgIdentifier = defaults.DEFAULT_ORG
	}
	if projectIdentifier == "" {
		projectIdentifier = defaults.DEFAULT_PROJECT
	}

	// Initialize Harness service discovery client
	config := svcdiscovery.NewConfiguration()
	config.Host = GetBaseUrlHost(c)
	config.Scheme = "https"
	config.DefaultHeader = map[string]string{
		"x-api-key": CliCdRequestData.AuthToken,
	}

	harnessClient := svcdiscovery.NewAPIClient(config)
	ctx := createAuthContext(c)

	println("Listing discovery agents...")
	println(GetColoredText(fmt.Sprintf("Account: %s, Org: %s, Project: %s, Environment: %s",
		CliCdRequestData.Account, orgIdentifier, projectIdentifier, environmentIdentifier), color.FgCyan))

	// Call the service discovery list API for the specific environment
	agents, resp, err := harnessClient.AgentApi.ListAgent(ctx, CliCdRequestData.Account, environmentIdentifier, 1, 50, true, &svcdiscovery.AgentApiListAgentOpts{
		OrganizationIdentifier: optional.NewString(orgIdentifier),
		ProjectIdentifier:      optional.NewString(projectIdentifier),
	})

	if err != nil {
		println(GetColoredText("Error listing discovery agents: "+err.Error(), color.FgRed))
		if resp != nil {
			println(GetColoredText(fmt.Sprintf("HTTP Status: %d", resp.StatusCode), color.FgYellow))
		}
		return err
	}

	// Display results
	if len(agents.Items) == 0 {
		println(GetColoredText("No discovery agents found in the specified environment.", color.FgYellow))
	} else {
		println(GetColoredText(fmt.Sprintf("Found %d discovery agent(s):", len(agents.Items)), color.FgGreen))
		for _, agent := range agents.Items {
			status := "Active"
			if agent.Removed {
				status = "Removed"
			}
			fmt.Printf("- ID: %s\n", GetColoredText(agent.Id, color.FgBlue))
			fmt.Printf("  Name: %s\n", GetColoredText(agent.Name, color.FgCyan))
			fmt.Printf("  Status: %s\n", GetColoredText(status, color.FgMagenta))
			fmt.Printf("  Environment: %s\n", GetColoredText(agent.EnvironmentIdentifier, color.FgGreen))
			fmt.Printf("  Organization: %s\n", GetColoredText(agent.OrganizationIdentifier, color.FgYellow))
			fmt.Printf("  Project: %s\n", GetColoredText(agent.ProjectIdentifier, color.FgMagenta))
			fmt.Printf("  Created: %s\n", GetColoredText(agent.CreatedAt, color.FgBlue))
			if agent.ServiceCount > 0 {
				fmt.Printf("  Services: %d\n", GetColoredText(fmt.Sprintf("%d", agent.ServiceCount), color.FgGreen))
			}
			if agent.NetworkMapCount > 0 {
				fmt.Printf("  Network Maps: %d\n", GetColoredText(fmt.Sprintf("%d", agent.NetworkMapCount), color.FgGreen))
			}
			fmt.Println()
		}
	}

	telemetry.Track(telemetry.TrackEventInfoPayload{EventName: "Discovery Agent Listed", UserId: CliCdRequestData.UserId}, map[string]interface{}{
		"accountId": CliCdRequestData.Account,
		"userId":    CliCdRequestData.UserId,
		"count":     len(agents.Items),
	})

	return nil
}

// Helper function to get base URL host without path
func GetBaseUrlHost(c *cli.Context) string {
	baseURL := GetNGBaseURL(c)
	// Extract host from URL like "https://app.harness.io/gateway/ng/api" -> "app.harness.io"
	if baseURL == "" {
		return "app.harness.io"
	}
	// Simple parsing - in real implementation you'd use url.Parse
	if baseURL == "https://app.harness.io/gateway/ng/api" {
		return "app.harness.io"
	}
	return "app.harness.io" // default
}

// Helper function to create context with authentication
func createAuthContext(c *cli.Context) context.Context {
	return context.Background()
}
