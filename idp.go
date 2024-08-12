package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	_ "github.com/fatih/color"
	"github.com/urfave/cli/v2"
)

var currentDirectory = filepath.Base(os.Getwd())
var branch = "main"

const yamlContentTemplate = `
apiVersion: backstage.io/v1alpha1
kind: Component
metadata:
  name: {repo_name}
  tags:
    - auto-generated
  annotations:
    backstage.io/source-location: url:{repo_path}
    github.com/project-slug: {project_slug}
spec:
  type: service
  lifecycle: experimental
  owner: Harness_Account_All_Users
  system: {orgName}
`

func getRepositoriesAPI(organization, token, repoPattern string) ([]map[string]string, error) {
	url := fmt.Sprintf("https://api.github.com/orgs/%s/repos", organization)
	headers := map[string]string{
		"Accept":               "application/vnd.github.v3+json",
		"Authorization":        fmt.Sprintf("Bearer %s", token),
		"X-GitHub-Api-Version": "2022-11-28",
	}

	var allReposInfo []map[string]string
	page := 1
	for {
		params := fmt.Sprintf("?page=%d&per_page=100", page)
		req, err := http.NewRequest("GET", url+params, nil)
		if err != nil {
			return nil, err
		}

		for key, value := range headers {
			req.Header.Set(key, value)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("unable to fetch repositories from page %d", page)
		}

		var repos []map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			break
		}

		for _, repo := range repos {
			repoName := strings.ToLower(repo["name"].(string))
			if repoName == currentDirectory {
				continue
			}
			repoPath := repo["html_url"].(string)
			if repoPattern == "" || regexp.MustCompile(repoPattern).MatchString(repoName) {
				allReposInfo = append(allReposInfo, map[string]string{"name": repoName, "html_url": repoPath})
			}
		}
		page++
	}

	return allReposInfo, nil
}

func listRepositories(c *cli.Context) error {
	organization := c.String("org")
	token := c.String("token")
	repoPattern := c.String("repo-pattern")

	yamlFilesCreated := 0
	fmt.Printf("Repositories in %s:\n", organization)

	repos, err := getRepositoriesAPI(organization, token, repoPattern)
	if err != nil {
		return err
	}
	for _, repo := range repos {
		repoName := strings.ToLower(repo["name"])
		if repoName == currentDirectory {
			continue
		}
		repoPath := repo["html_url"]
		if repoPattern == "" || regexp.MustCompile(repoPattern).MatchString(repoName) {
			fmt.Println(repoName)
			createOrUpdateCatalogInfo(organization, repoName, repoPath)
			yamlFilesCreated++
		}
	}
	fmt.Println("----------")
	fmt.Printf("Total YAML files created or updated: %d\n", yamlFilesCreated)
	return nil
}

func createOrUpdateCatalogInfo(organization, repoName, repoPath string) {
	directory := fmt.Sprintf("services/%s", repoName)
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		os.MkdirAll(directory, os.ModePerm)
	}

	yamlFilePath := fmt.Sprintf("%s/catalog-info.yaml", directory)
	content := fmt.Sprintf(yamlContentTemplate, repoName, repoPath, fmt.Sprintf("%s/%s", organization, repoName), organization)

	if _, err := os.Stat(yamlFilePath); err == nil {
		ioutil.WriteFile(yamlFilePath, []byte(content), 0644)
	} else {
		ioutil.WriteFile(yamlFilePath, []byte(content), 0644)
	}
}

func registerYamls(c *cli.Context) error {
	organization := c.String("org")
	account := c.String("account")
	xApiKey := c.String("x-api-key")

	fmt.Println("Registering YAML files...")
	count := 0
	apiURL := fmt.Sprintf("https://idp.harness.io/%s/idp/api/catalog/locations", account)

	repos, err := ioutil.ReadDir("./services")
	if err != nil {
		return err
	}

	for _, repo := range repos {
		if repo.IsDir() && repo.Name() != currentDirectory {
			directory := fmt.Sprintf("services/%s", repo.Name())
			apiPayload := map[string]string{
				"target": fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s/catalog-info.yaml", organization, currentDirectory, branch, directory),
				"type":   "url",
			}
			apiHeaders := map[string]string{
				"x-api-key":       xApiKey,
				"Content-Type":    "application/json",
				"Harness-Account": account,
			}

			payloadBytes, err := json.Marshal(apiPayload)
			if err != nil {
				return err
			}

			req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(payloadBytes))
			if err != nil {
				return err
			}
			for key, value := range apiHeaders {
				req.Header.Set(key, value)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode == 200 || resp.StatusCode == 201 {
				fmt.Printf("Location registered for file: %s\n", repo.Name())
				count++
			} else if resp.StatusCode == 409 {
				refreshPayload := map[string]string{"entityRef": fmt.Sprintf("component:default/%s", repo.Name())}
				refreshURL := fmt.Sprintf("https://idp.harness.io/%s/idp/api/catalog/refresh", account)
				payloadBytes, _ := json.Marshal(refreshPayload)
				req, _ := http.NewRequest("POST", refreshURL, bytes.NewBuffer(payloadBytes))
				for key, value := range apiHeaders {
					req.Header.Set(key, value)
				}
				resp, _ := client.Do(req)
				fmt.Printf("Location already exists for file: %s. Refreshing it\n", repo.Name())
				count++
			} else {
				fmt.Printf("Failed to register location for file: %s. Status code: %d\n", repo.Name(), resp.StatusCode)
			}
		}
	}
	return nil
}

func pushYamls() error {
	fmt.Println("Pushing YAMLs...")
	if err := runCommand("git", "add", "services/"); err != nil {
		return err
	}
	if err := runCommand("git", "commit", "-m", "Adding YAMLs"); err != nil {
		return err
	}
	return runCommand("git", "push")
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
