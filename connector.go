package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	"os"
)

type connectorDetails struct {
	Name              string `yaml:"name"`
	Identifier        string `yaml:"identifier"`
	Description       string `yaml:"description"`
	OrgIdentifier     string `yaml:"orgIdentifier"`
	ProjectIdentifier string `yaml:"projectIdentifier"`
	Type              string `yaml:"type"`
	// Add nested fields from the yaml here
}

// apply(create or update) connector
func applyConnector(c *cli.Context) error {
	fmt.Println("File path: ", c.String("file"))
	fmt.Println("Trying to create or update a connector using the connector yaml")

	var connectorDetails connectorDetails
	connectorDetails.getConfiguration()

	fmt.Println("Connector yaml details", connectorDetails)
	return nil
}

// Delete an existing connector
func deleteConnector(*cli.Context) error {
	return nil
}

// Delete an existing connector
func listConnector(*cli.Context) error {
	return nil
}

// Get the configuration for connector from yaml
func (c *connectorDetails) getConfiguration() *connectorDetails {

	y, err := os.OpenFile(cliCdReq.File, os.O_RDONLY, 0644)
	if err != nil {
		fmt.Printf("yamlFile.Get err   #%v ", err)
	}
	buffer := make([]byte, 6536)
	yamlFile, err := y.ReadFrom(y)
	//ch := connectorDetails{}

	err = yaml.Unmarshal(buffer[:yamlFile], c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	fmt.Println("Unmarshaled data:", c)
	return c
}
