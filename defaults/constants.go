package defaults

const API_VERSION = "v1"
const AWS_CROSS_ACCOUNT_ROLE_ARN = "ROLE_ARN"
const AWS_ACCESS_KEY = "AWS_ACCESS_KEY"
const AWS_SECRET_IDENTIFIER = "awssecret"
const ACCOUNTS_ENDPOINT = "accounts/"
const BUCKET_NAME_PLACEHOLDER = "CLOUD_BUCKET_NAME"
const CONNECTOR_ENDPOINT = "connectors"
const CONTENT_TYPE_JSON = "application/json"
const CONTENT_TYPE_YAML = "application/yaml"
const DEFAULT_PROJECT = "default_project"
const DEFAULT_ORG = "default"
const DOCKER_USERNAME_PLACEHOLDER = "DOCKER_USERNAME"
const ENVIRONMENT_ENDPOINT = "environmentsV2"
const GITHUB_SECRET_IDENTIFIER = "harness_gitpat"
const GITOPS_AGENT_IDENTIFIER_PLACEHOLDER = "AGENT_NAME"
const GITOPS_APPLICATION_ENDPOINT = "applications"
const GITOPS_BASE_URL = "/gateway/gitops/api/v1/agents/"
const GITOPS_CLUSTER_ENDPOINT = "clusters"
const GITOPS_ENDPOINT = "/gitops"
const GITOPS_REPOSITORY_ENDPOINT = "repositories"
const GCP_SECRET_IDENTIFIER = "gcpsecret"
const HOST_IP_PLACEHOLDER = "HOST_IP_OR_FQDN"
const HOST_PORT_PLACEHOLDER = "PORT"
const INSTANCE_NAME_PLACEHOLDER = "INSTANCE_NAME"
const NG_BASE_URL = "/gateway/ng/api"
const ORGANIZATIONS_ENDPOINT = "orgs/"
const PROJECTS_ENDPOINT = "projects/"
const PROJECT_NAME_PLACEHOLDER = "CLOUD_PROJECT_NAME"
const REGION_NAME_PLACEHOLDER = "CLOUD_REGION_NAME"
const SSH_PRIVATE_KEY_SECRET_IDENTIFIER = "harness_sshprivatekey"
const SSH_KEY_FILE_SECRET_IDENTIFIER = "harness_sshsecretfile"
const WINRM_SECRET_IDENTIFIER = "harness_winrmpwd"
const WINRM_PASSWORD_SECRET_IDENTIFIER = "winrm_passwd"
const INFRA_ENDPOINT = "infrastructures"
const HARNESS_UX_VERSION = "ng"

// Enum for multiple platforms
const (
	GCP string = "GCP"
	AWS        = "AWS"
)

const NOT_IMPLEMENTED = "Command Not_Implemented. Check back later.."
const EXECUTE_PIPELINE_ENDPOINT = "execute"
const PIPELINES_BASE_URL = "/gateway/pipeline/api"
const PIPELINES_ENDPOINT = "pipelines"
const PIPELINES_ENDPOINT_V2 = "pipelines/v2"
const SECRETS_STORE_PATH = ".secrets.json"
const SERVICES_ENDPOINT = "servicesV2"
const SECRETS_ENDPOINT = "v2/secrets"
const SECRETS_ENDPOINT_WITH_IDENTIFIER = "v2/secrets/%s"
const FILE_SECRETS_ENDPOINT = "v2/secrets/files/%s"
const USER_INFO_ENDPOINT = "user/currentUser"
const GITHUB_USERNAME_PLACEHOLDER = "GITHUB_USERNAME"
const DELEGATE_NAME_PLACEHOLDER = "DELEGATE_NAME"
const GITHUB_PAT_PLACEHOLDER = "GITHUB-PAT"
const HARNESS_PROD_URL = "https://app.harness.io/"
