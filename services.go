package main

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func applyService(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create or update a service using the service yaml.")

	// Getting the account details
	reqUrl := GetUrlWithQueryParams("", "", "services", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))
	//requestBody := getJsonFromYaml(content)
	requestBody := &map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(content), requestBody); err != nil {
		return err
	}

	fmt.Println("reqUrl: ", reqUrl)
	fmt.Println("requestBody: ", requestBody)
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

func deleteService() {
	fmt.Println(NOT_IMPLEMENTED)
}
