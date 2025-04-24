package ar

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"github.com/olekukonko/tablewriter"
	"context"
	"github.com/go-resty/resty/v2"
)

// Helper function to initialize API client
func NewAPIClient(baseURL string) *ClientWithResponses {
	client := resty.New()
	client.SetBaseURL(baseURL)

	return &ClientWithResponses{}
}

// Wrapper for handling CLI operations and output formatting
// This file is not auto-generated and will need to be manually updated
// to add new operations or modify existing ones
