package main

import (
	"fmt"
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
	reqUrl := GetUrlWithQueryParams("", "", "connectors", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(filePath)
	//requestBody := getJsonFromYaml(content)
	requestBody := &map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return err
	}

	fmt.Println("reqUrl: ", reqUrl)
	fmt.Println("requestBody: ", requestBody)
	//resp, err := Post(reqUrl, cliCdRequestData.AuthToken, body, "text/yaml")
	resp, err := Post(reqUrl, cliCdRequestData.AuthToken, requestBody, JSON_CONTENT_TYPE)

	fmt.Println("Response Headers: ", resp)
	if err == nil {
		fmt.Printf("Response status: %s \n", resp.Status)
		fmt.Printf("Response code: %s \n", resp.Code)
		fmt.Printf("Response resource: %s \n", resp.Resource)
		fmt.Printf("Response messages: %s \n", resp.Messages)
		printJson(resp.Data)
		return nil
	}

	//fmt.Println("Connector yaml details", connectorDetails)
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
