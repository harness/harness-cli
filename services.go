package main

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

func applyService(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create or update a service using the service yaml.")
	createOrUpdateSvcURL := GetUrlWithQueryParams("", NG_BASE_URL, "servicesV2", map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content = readFromFile(c.String("file"))
	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid yaml", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "service", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "service", "name").(string))
	projectIdentifier := "default_project"
	orgIdentifier := "default"
	//setup payload for svc create / update
	svcPayload := HarnessService{Identifier: identifier, Name: name, ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := getEntity(fmt.Sprintf("servicesV2/%s", identifier), projectIdentifier, orgIdentifier, map[string]string{})
	var resp ResponseBody
	var err error
	if !entityExists {
		println("Creating service with id: ", getColoredText(identifier, color.FgGreen))
		fmt.Println("createOrUpdateSvcURL: ", createOrUpdateSvcURL)
		fmt.Println("requestBody: ", requestBody)
		resp, err = Post(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, JSON_CONTENT_TYPE)
		if err == nil {
			println(getColoredText("Service created successfully!", color.FgGreen))
			printJson(resp.Data)
			return nil
		}
	} else {
		println("Found service with id: ", getColoredText(identifier, color.FgGreen))
		println(getColoredText("Updating service details....", color.FgGreen))
		resp, err = Put(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, JSON_CONTENT_TYPE)
		if err == nil {
			println(getColoredText("Service updated successfully!", color.FgGreen))
			//printJson(resp.Data)
			return nil
		}
	}

	return nil
}

func deleteService() {
	fmt.Println(NOT_IMPLEMENTED)
}
