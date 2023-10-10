package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/utils"
	"regexp"
	"strings"
)

// apply(create or update) connector
func applyConnector(c *cli.Context) error {
	filePath := c.String("file")
	githubUsername := c.String("git-user")
	delegateName := c.String("delegate-name")
	awsCrossAccountRoleArn := c.String("aws-cross-account-role-arn")
	awsAccessKey := c.String("aws-access-key")
	awsRegion := c.String("cloud-region")
	hostIpOrFqdn := c.String("host-ip")
	hostPort := c.String("port")

	baseURL := GetNGBaseURL(c)

	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update connector using the yaml=",
		GetColoredText(filePath, color.FgCyan))

	// Getting the account details
	createConnectorURL := GetUrlWithQueryParams("", baseURL, defaults.CONNECTOR_ENDPOINT, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})

	var content, _ = ReadFromFile(filePath)
	if isGithubConnectorYAML(content) {
		if githubUsername == "" || githubUsername == defaults.GITHUB_USERNAME_PLACEHOLDER {
			githubUsername = TextInput("Enter valid github username:")
		}
		content = ReplacePlaceholderValues(content, defaults.GITHUB_USERNAME_PLACEHOLDER, githubUsername)
	}
	if isK8sConnectorYAML(content) {
		if delegateName == "" || delegateName == defaults.DELEGATE_NAME_PLACEHOLDER {
			delegateName = TextInput("Enter valid delegate name:")
		}
		content = ReplacePlaceholderValues(content, defaults.DELEGATE_NAME_PLACEHOLDER, delegateName)
	}
	if isAwsConnectorYAML(content) {
		if awsCrossAccountRoleArn == "" || awsCrossAccountRoleArn == defaults.AWS_CROSS_ACCOUNT_ROLE_ARN {
			awsCrossAccountRoleArn = TextInput("Enter valid aws cross account role arn:")
		}
		if awsAccessKey == "" || awsAccessKey == defaults.AWS_ACCESS_KEY {
			awsAccessKey = TextInput("Enter valid aws access key:")
		}
		if awsRegion == "" || awsRegion == defaults.REGION_NAME_PLACEHOLDER {
			awsRegion = TextInput("Enter valid aws region:")
		}
		if delegateName == "" || delegateName == defaults.DELEGATE_NAME_PLACEHOLDER {
			delegateName = TextInput("Enter valid delegate name:")
		}

		//TODO: find a better way to resolve placeholders, dont depend on fixed placeholders
		content = ReplacePlaceholderValues(content, defaults.AWS_CROSS_ACCOUNT_ROLE_ARN, awsCrossAccountRoleArn)
		content = ReplacePlaceholderValues(content, defaults.AWS_ACCESS_KEY, awsAccessKey)
		content = ReplacePlaceholderValues(content, defaults.REGION_NAME_PLACEHOLDER, awsRegion)
		content = ReplacePlaceholderValues(content, defaults.DELEGATE_NAME_PLACEHOLDER, delegateName)
	}
	if isPdcConnectorYAML(content) {
		hasPortNumber := strings.Contains(content, defaults.HOST_PORT_PLACEHOLDER)
		if hostIpOrFqdn == "" || hostIpOrFqdn == defaults.HOST_IP_PLACEHOLDER {
			hostIpOrFqdn = TextInput("Enter valid host ip / fqdn:")
		}
		if hasPortNumber && (hostPort == "" || hostPort == defaults.HOST_PORT_PLACEHOLDER) {
			hostPort = TextInput("Enter valid host port:")
		}
		if delegateName == "" || delegateName == defaults.DELEGATE_NAME_PLACEHOLDER {
			delegateName = TextInput("Enter valid delegate name:")
		}
		content = ReplacePlaceholderValues(content, defaults.HOST_IP_PLACEHOLDER, hostIpOrFqdn)
		content = ReplacePlaceholderValues(content, defaults.DELEGATE_NAME_PLACEHOLDER, delegateName)
		content = ReplacePlaceholderValues(content, defaults.HOST_PORT_PLACEHOLDER, hostPort)
	}
	requestBody := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return err
	}
	identifier := ValueToString(GetNestedValue(requestBody, "connector", "identifier").(string))
	projectIdentifier := ValueToString(GetNestedValue(requestBody, "connector", "projectIdentifier").(string))
	orgIdentifier := ValueToString(GetNestedValue(requestBody, "connector", "orgIdentifier").(string))
	entityExists := GetEntity(baseURL, fmt.Sprintf("connectors/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{})

	var err error
	if !entityExists {
		println("Creating connector with id: ", GetColoredText(identifier, color.FgGreen))
		_, err = client.Post(createConnectorURL, CliCdRequestData.AuthToken, requestBody, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created connector with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.CONNECTOR_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":     CliCdRequestData.Account,
				"connectorType": GetTypeFromYAML(content),
			})
			return nil
		}

	} else {
		println("Found connector with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of connector with id=", GetColoredText(identifier, color.FgBlue))

		_, err = client.Put(createConnectorURL, CliCdRequestData.AuthToken, requestBody, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully updated connector with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.CONNECTOR_UPDATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":     CliCdRequestData.Account,
				"connectorType": GetTypeFromYAML(content),
				"userId":        CliCdRequestData.UserId,
			})
			return nil
		}

	}
	return nil
}

func isGithubConnectorYAML(str string) bool {
	regexPattern := `type:\s+Github`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

func isK8sConnectorYAML(str string) bool {
	regexPattern := `type:\s+K8sCluster`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

func isAwsConnectorYAML(str string) bool {
	regexPattern := `type:\s+Aws`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

func isPdcConnectorYAML(str string) bool {
	regexPattern := `type:\s+Pdc`
	match, _ := regexp.MatchString(regexPattern, str)
	return match
}

// Delete an existing connector
func deleteConnector(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}

// Delete an existing connector
func listConnector(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}
