package main

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var gcpBucketName = ""

func applyService(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := getNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	gcpProjectName = c.String("gcp-project")
	gcpBucketName = c.String("gcp-bucket")
	fmt.Println("Trying to create or update service using the yaml=",
		getColoredText(filePath, color.FgCyan))
	createOrUpdateSvcURL := GetUrlWithQueryParams("", baseURL, SERVICES_ENDPOINT, map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
	})
	var content, _ = readFromFile(c.String("file"))
	content = updateServiceYamlContent(content)

	requestBody := getJsonFromYaml(content)
	if requestBody == nil {
		println(getColoredText("Please enter valid yaml", color.FgRed))
	}
	identifier := valueToString(GetNestedValue(requestBody, "service", "identifier").(string))
	name := valueToString(GetNestedValue(requestBody, "service", "name").(string))
	//setup payload for svc create / update
	svcPayload := HarnessService{Identifier: identifier, Name: name,
		ProjectIdentifier: DEFAULT_PROJECT, OrgIdentifier: DEFAULT_ORG, Yaml: content}
	entityExists := getEntity(baseURL, fmt.Sprintf("servicesV2/%s", identifier),
		DEFAULT_PROJECT, DEFAULT_ORG, map[string]string{})
	var err error
	if !entityExists {
		println("Creating service with id: ", getColoredText(identifier, color.FgGreen))
		_, err = Post(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Successfully created service with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	} else {
		println("Found service with id=", getColoredText(identifier, color.FgCyan))
		println("Updating details of service with id=", getColoredText(identifier, color.FgBlue))
		_, err = Put(createOrUpdateSvcURL, cliCdRequestData.AuthToken, svcPayload, CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(getColoredText("Successfully updated connector with id= ", color.FgGreen) +
				getColoredText(identifier, color.FgBlue))
			return nil
		}
	}

	return nil
}

func updateServiceYamlContent(content string) string {
	var serviceType = fetchCloudType(content)
	switch {
	case serviceType == GCP:
		log.Info("Looks like you are creating a service for GCP," +
			" validating GCP project and bucket now...")
		if gcpProjectName == "" || gcpProjectName == GCP_PROJECT_NAME_PLACEHOLDER {
			gcpProjectName = TextInput("Enter a valid GCP project name:")
		}

		if gcpBucketName == "" || gcpBucketName == GCP_BUCKET_NAME_PLACEHOLDER {
			gcpBucketName = TextInput("Enter a valid GCP bucket name:")
		}
		log.Info("Got your gcp project and bucket info, let's create the service now...")
		content = replacePlaceholderValues(content, GCP_PROJECT_NAME_PLACEHOLDER, gcpProjectName)
		content = replacePlaceholderValues(content, GCP_BUCKET_NAME_PLACEHOLDER, gcpBucketName)
	case serviceType == AWS:
		log.Info("Looks like you are creating a service for AWS, validating yaml now...")
	default:
		return content
	}

	return content
}

func deleteService(*cli.Context) error {
	fmt.Println(NOT_IMPLEMENTED)
	return nil
}
