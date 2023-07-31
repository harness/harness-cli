package main

const CONNECTOR_ENDPOINT = "connectors"
const CONTENT_TYPE_JSON = "application/json"
const CONTENT_TYPE_YAML = "application/yaml"
const ENVIRONMENT_ENDPOINT = "environmentsV2"
const GCP_PROJECT_NAME_PLACEHOLDER = "GCP_PROJECT_NAME"
const GCP_REGION_NAME_PLACEHOLDER = "GCP_REGION_NAME"
const GITHUB_SECRET_IDENTIFIER = "harness_gitpat"
const NG_BASE_URL = "/gateway/ng/api"
const DEFAULT_PROJECT = "default_project"
const DEFAULT_ORG = "default"
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
const AWS_CROSS_ACCOUNT_ROLE_ARN = "<Add the ARN for Your Role>"
const AWS_ACCESS_KEY = "<Add the Access Key for your Region>"
const AWS_SECRET_KEY = "awsaccesskey"
const AWS_REGION = "<Specifiy the Region in which you created the Users and the Roles>"
const DELEGATE_NAME_PLACEHOLDER = "DELEGATE_NAME"
const GITHUB_PAT_PLACEHOLDER = "GITHUB-PAT"
const HARNESS_PROD_URL = "https://app.harness.io/"
