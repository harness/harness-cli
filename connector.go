package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// apply(create or update) connector
func applyConnector(c *cli.Context) error {
	filePath := c.String("file")
	githubUsername := c.String("git-user")
	delegateName := c.String("delegate-name")

	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("Trying to create or update connector using the yaml=",
		getColoredText(filePath, color.FgCyan))

	// Getting the account details
	createConnectorURL := GetUrlWithQueryParams("", NG_BASE_URL, CONNECTOR_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})

	var content = readFromFile(filePath)
	if isGithubConnectorYAML(content) {
		if githubUsername == "" || githubUsername == GITHUB_USERNAME_PLACEHOLDER {
			githubUsername = TextInput("Enter valid github username:")

		}
		content = replacePlaceholderValues(content, GITHUB_USERNAME_PLACEHOLDER, githubUsername)
	}
	if isK8sConnectorYAML(content) {
		if delegateName == "" || delegateName == DELEGATE_NAME_PLACEHOLDER {
			delegateName = TextInput("Enter valid delegate name:")
		}
		content = replacePlaceholderValues(content, DELEGATE_NAME_PLACEHOLDER, delegateName)
	}
	requestBody := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return err
	}
	printJson(requestBody)
	identifier := valueToString(GetNestedValue(requestBody, "connector", "identifier").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "connector", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "connector", "orgIdentifier").(string))
	entityExists := getEntity(NG_BASE_URL, fmt.Sprintf("connectors/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{})

	var err error
	if !entityExists {
		println("Creating connector with id: ", getColoredText(identifier, color.FgGreen))
		_, err = Post(createConnectorURL, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully created connector with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))

			return nil
		}

	} else {
		println("Found connector with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of connector with id=", getColoredText(identifier, color.FgBlue))

		_, err = Put(createConnectorURL, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Successfully updated connector with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}

	}
	return nil
}

func replacePlaceholderValues(haystack string, needle string, value string) string {
	return strings.ReplaceAll(haystack, needle, value)
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

// Delete an existing connector
func deleteConnector(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}

// Delete an existing connector
func listConnector(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
