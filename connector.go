package main

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// apply(create or update) connector
func applyConnector(c *cli.Context) error {
	filePath := c.String("file")
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	fmt.Println("File path: ", filePath)
	fmt.Println("Trying to create or update a connector using the provided connector yaml")

	// Getting the account details
	createConnectorURL := GetUrlWithQueryParams("", NG_BASE_URL, "connectors", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})

	var content = readFromFile(filePath)

	requestBody := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return err
	}

	identifier := valueToString(GetNestedValue(requestBody, "connector", "identifier").(string))
	projectIdentifier := valueToString(GetNestedValue(requestBody, "connector", "projectIdentifier").(string))
	orgIdentifier := valueToString(GetNestedValue(requestBody, "connector", "orgIdentifier").(string))
	entityExists := getEntity(NG_BASE_URL, fmt.Sprintf("connectors/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{})
	var resp ResponseBody
	var err error
	if !entityExists {

		println("Creating connector with id: ", getColoredText(identifier, color.FgGreen))
		resp, err = Post(createConnectorURL, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Connector created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found connector with id: ", getColoredText(identifier, color.FgGreen))

		println(getColoredText("Updating connector details....", color.FgGreen))
		resp, err = Put(createConnectorURL, cliCdRequestData.AuthToken, requestBody, CONTENT_TYPE_JSON)
		if err == nil {
			println(getColoredText("Connector updated successfully!", color.FgGreen))
			return nil
		}

	}
	return nil
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
