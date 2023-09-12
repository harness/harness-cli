package main

const AWS_CROSS_ACCOUNT_ROLE_ARN = "ROLE_ARN"
const AWS_ACCESS_KEY = "AWS_ACCESS_KEY"
const AWS_SECRET_IDENTIFIER = "awssecret"
const BUCKET_NAME_PLACEHOLDER = "CLOUD_BUCKET_NAME"
const CONNECTOR_ENDPOINT = "connectors"
const CONTENT_TYPE_JSON = "application/json"
const CONTENT_TYPE_YAML = "application/yaml"
const DEFAULT_PROJECT = "default_project"
const DEFAULT_ORG = "default"
const ENVIRONMENT_ENDPOINT = "environmentsV2"
const GITHUB_SECRET_IDENTIFIER = "harness_gitpat"
const GITOPS_AGENT_IDENTIFIER_PLACEHOLDER = "AGENT_NAME"
const GITOPS_BASE_URL = "/gateway/gitops/api/v1/agents/"
const GITOPS_CLUSTER_ENDPOINT = "clusters"
const GITOPS_REPOSITORY_ENDPOINT = "repositories"
const GCP_SECRET_IDENTIFIER = "gcpsecret"
const NG_BASE_URL = "/gateway/ng/api"
const PROJECT_NAME_PLACEHOLDER = "CLOUD_PROJECT_NAME"
const REGION_NAME_PLACEHOLDER = "CLOUD_REGION_NAME"
const INFRA_ENDPOINT = "infrastructures"

// Enum for multiple platforms
const (
	GCP string = "GCP"
	AWS        = "AWS"
)

const NOT_IMPLEMENTED = "Command Not_Implemented. Check back later.."
const PIPELINES_BASE_URL = "/gateway/pipeline/api"
const PIPELINES_ENDPOINT = "pipelines"
const PIPELINES_ENDPOINT_V2 = "pipelines/v2"
const SECRETS_STORE_PATH = ".secrets.json"
const SERVICES_ENDPOINT = "servicesV2"
const GITHUB_USERNAME_PLACEHOLDER = "GITHUB_USERNAME"
const DELEGATE_NAME_PLACEHOLDER = "DELEGATE_NAME"
const GITHUB_PAT_PLACEHOLDER = "GITHUB-PAT"
const HARNESS_PROD_URL = "https://app.harness.io/"
