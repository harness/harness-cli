package main

import (
	"fmt"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	"harness/telemetry"
	. "harness/types"
	. "harness/utils"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var cloudBucketName = ""

func applyService(c *cli.Context) error {
	filePath := c.String("file")
	baseURL := GetNGBaseURL(c)
	if filePath == "" {
		fmt.Println("Please enter valid filename")
		return nil
	}
	cloudProjectName = c.String("cloud-project")
	cloudBucketName = c.String("cloud-bucket")
	cloudRegionName = c.String("cloud-region")
	orgIdentifier := c.String("org-id")
	projectIdentifier := c.String("project-id")
	fmt.Println("Trying to create or update service using the yaml=",
		GetColoredText(filePath, color.FgCyan))
	createOrUpdateSvcURL := GetUrlWithQueryParams("", baseURL, defaults.SERVICES_ENDPOINT, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})
	var content, _ = ReadFromFile(c.String("file"))
	content = updateServiceYamlContent(content)

	requestBody := GetJsonFromYaml(content)
	if requestBody == nil {
		println(GetColoredText("Please enter valid yaml", color.FgRed))
	}
	identifier := ValueToString(GetNestedValue(requestBody, "service", "identifier").(string))
	name := ValueToString(GetNestedValue(requestBody, "service", "name").(string))

	if orgIdentifier == "" {
		orgIdentifier = defaults.DEFAULT_ORG
	}
	if projectIdentifier == "" {
		projectIdentifier = defaults.DEFAULT_PROJECT
	}
	//setup payload for svc create / update
	svcPayload := HarnessService{Identifier: identifier, Name: name,
		ProjectIdentifier: projectIdentifier, OrgIdentifier: orgIdentifier, Yaml: content}
	entityExists := GetEntity(baseURL, fmt.Sprintf("servicesV2/%s", identifier),
		projectIdentifier, orgIdentifier, map[string]string{})
	var err error
	if !entityExists {
		println("Creating service with id: ", GetColoredText(identifier, color.FgGreen))
		_, err = client.Post(createOrUpdateSvcURL, CliCdRequestData.AuthToken, svcPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully created service with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SVC_CREATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId":     CliCdRequestData.Account,
				"connectorType": GetTypeFromYAML(content),
				"userId":        CliCdRequestData.UserId,
			})
			return nil
		}
	} else {
		println("Found service with id=", GetColoredText(identifier, color.FgCyan))
		println("Updating details of service with id=", GetColoredText(identifier, color.FgBlue))
		_, err = client.Put(createOrUpdateSvcURL, CliCdRequestData.AuthToken, svcPayload, defaults.CONTENT_TYPE_JSON, nil)
		if err == nil {
			println(GetColoredText("Successfully updated connector with id= ", color.FgGreen) +
				GetColoredText(identifier, color.FgBlue))
			telemetry.Track(telemetry.TrackEventInfoPayload{EventName: telemetry.SVC_UPDATED, UserId: CliCdRequestData.UserId}, map[string]interface{}{
				"accountId": CliCdRequestData.Account,
				"type":      GetTypeFromYAML(content),
				"userId":    CliCdRequestData.UserId,
			})
			return nil
		}
	}

	return nil
}

func updateServiceYamlContent(content string) string {
	var serviceType = strings.ToLower(FetchCloudType(content))
	switch {
	case strings.EqualFold(serviceType, defaults.GCP):
		log.Info("Looks like you are creating a service for GCP," +
			" validating GCP project and bucket now...")
		if cloudProjectName == "" || cloudProjectName == defaults.PROJECT_NAME_PLACEHOLDER {
			cloudProjectName = TextInput("Enter a valid GCP project name:")
		}

		if cloudBucketName == "" || cloudBucketName == defaults.BUCKET_NAME_PLACEHOLDER {
			cloudBucketName = TextInput("Enter a valid GCP bucket name:")
		}
		log.Info("Got your gcp project and bucket info, let's create the service now...")
		content = ReplacePlaceholderValues(content, defaults.PROJECT_NAME_PLACEHOLDER, cloudProjectName)
		content = ReplacePlaceholderValues(content, defaults.BUCKET_NAME_PLACEHOLDER, cloudBucketName)
		return content
	case strings.EqualFold(serviceType, defaults.AWS):

		log.Info("Looks like you are creating a service for AWS, validating yaml now...")
		hasRegionName := strings.Contains(content, defaults.REGION_NAME_PLACEHOLDER)
		hasBucketName := strings.Contains(content, defaults.BUCKET_NAME_PLACEHOLDER)
		if hasRegionName && (cloudRegionName == "" || cloudRegionName == defaults.REGION_NAME_PLACEHOLDER) {
			cloudRegionName = TextInput("Enter a valid AWS region name:")
		}

		if hasBucketName && (cloudBucketName == "" || cloudBucketName == defaults.BUCKET_NAME_PLACEHOLDER) {
			cloudBucketName = TextInput("Enter a valid AWS bucket name:")
		}
		log.Info("Got your aws project and bucket info, let's create the service now...")
		content = ReplacePlaceholderValues(content, defaults.REGION_NAME_PLACEHOLDER, cloudRegionName)
		content = ReplacePlaceholderValues(content, defaults.BUCKET_NAME_PLACEHOLDER, cloudBucketName)
		return content
	default:
		return content
	}

	return content
}

func deleteService(*cli.Context) error {
	fmt.Println(defaults.NOT_IMPLEMENTED)
	return nil
}
