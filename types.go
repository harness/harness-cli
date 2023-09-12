package main

type EntityType string
type ImportType string
type StoreType int64

const (
	UNDEFINED StoreType = iota
	INLINE
	REMOTE
)

type Filter struct {
	Type           ImportType `json:"importType"`
	AppId          string     `json:"appId"`
	TriggerIds     []string   `json:"triggerIds"`
	WorkflowIds    []string   `json:"workflowIds"`
	PipelineIds    []string   `json:"pipelineIds"`
	Ids            []string   `json:"ids"`
	ServiceIds     []string   `json:"serviceIds"`
	EnvironmentIds []string   `json:"environmentIds"`
}

type DestinationDetails struct {
	AccountIdentifier string `json:"accountIdentifier"`
	AuthToken         string `json:"authToken"`
	ProjectIdentifier string `json:"projectIdentifier"`
	OrgIdentifier     string `json:"orgIdentifier"`
}

type EntityDefaults struct {
	Scope              string `json:"scope"`
	WorkflowAsPipeline bool   `json:"workflowAsPipeline"`
}

type Defaults struct {
	SecretManagerTemplate EntityDefaults `json:"SECRET_MANAGER_TEMPLATE"`
	SecretManager         EntityDefaults `json:"SECRET_MANAGER"`
	Secret                EntityDefaults `json:"SECRET"`
	Connector             EntityDefaults `json:"CONNECTOR"`
	Workflow              EntityDefaults `json:"WORKFLOW"`
	Template              EntityDefaults `json:"TEMPLATE"`
}

type Inputs struct {
	Defaults Defaults `json:"defaults"`
}

type OrgDetails struct {
	Identifier  string `json:"identifier"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ProjectDetails struct {
	OrgIdentifier string   `json:"orgIdentifier"`
	Identifier    string   `json:"identifier"`
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	Modules       []string `json:"modules"`
	Description   string   `json:"description"`
}

type BulkProjectResult struct {
	AppName           string       `json:"appName"`
	AppId             string       `json:"appId"`
	ProjectIdentifier string       `json:"projectIdentifier"`
	ProjectName       string       `json:"projectName"`
	Error             UpgradeError `json:"error"`
}

type BulkCreateBody struct {
	Org                  string `json:"orgIdentifier"`
	IdentifierCaseFormat string `json:"identifierCaseFormat"`
}

type ProjectBody struct {
	Project ProjectDetails `json:"project"`
}

type ProjectListBody struct {
	Projects []ProjectBody `json:"content"`
}

type OrgListBody struct {
	Organisations []OrgResponse `json:"content"`
}

type OrgResponse struct {
	Org OrgBody `json:"organizationResponse"`
}

type OrgBody struct {
	Org OrgDetails `json:"organization"`
}

type RequestBody struct {
	DestinationDetails   DestinationDetails `json:"destinationDetails"`
	EntityType           EntityType         `json:"entityType"`
	Filter               Filter             `json:"filter"`
	Inputs               Inputs             `json:"inputs"`
	IdentifierCaseFormat string             `json:"identifierCaseFormat"`
}

type CurrentGenEntity struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	AppId string `json:"appId"`
}

type SkipDetail struct {
	Entity CurrentGenEntity `json:"cgBasicInfo"`
	Reason string           `json:"reason"`
}

type UpgradeError struct {
	Message string           `json:"message"`
	Entity  CurrentGenEntity `json:"entity"`
}

type MigrationStats struct {
	SuccessfullyMigrated int64 `json:"successfullyMigrated"`
	AlreadyMigrated      int64 `json:"alreadyMigrated"`
}

type Resource struct {
	RequestId       string                    `json:"requestId"`
	Stats           map[string]MigrationStats `json:"stats"`
	Errors          []UpgradeError            `json:"errors"`
	Status          string                    `json:"status"`
	ResponsePayload interface{}               `json:"responsePayload"`
}

type ResponseBody struct {
	Code     interface{}        `json:"code"`
	Message  string             `json:"message"`
	Status   string             `json:"status"`
	Data     interface{}        `json:"data"`
	Resource interface{}        `json:"resource"`
	Messages []ResponseMessages `json:"responseMessages"`
}

type ResponseMessages struct {
	Code         string      `json:"code"`
	Level        string      `json:"level"`
	Message      string      `json:"message"`
	Exception    interface{} `json:"exception"`
	FailureTypes interface{} `json:"failureTypes"`
}

type SaveSummary struct {
	Stats                  map[string]MigrationStats `json:"stats"`
	Errors                 []UpgradeError            `json:"errors"`
	SkipDetails            []SkipDetail              `json:"skipDetails"`
	SkippedExpressionsList []SkippedExpressionDetail `json:"skippedExpressions"`
}

type SkippedExpressionDetail struct {
	EntityType        string   `json:"entityType"`
	Identifier        string   `json:"identifier"`
	OrgIdentifier     string   `json:"orgIdentifier"`
	ProjectIdentifier string   `json:"projectIdentifier"`
	Expressions       []string `json:"expressions"`
}

type SummaryResponse struct {
	Summary map[string]EntitySummary `json:"summary"`
}

type SummaryDetails struct {
	Count  int64  `json:"count"`
	Status string `json:"status"`
}

type EntitySummary struct {
	Name                     string                    `json:"name"`
	Count                    int64                     `json:"count"`
	TypeSummary              map[string]int64          `json:"typeSummary"`
	TypesSummary             map[string]SummaryDetails `json:"typesSummary"`
	StepTypeSummary          map[string]int64          `json:"stepTypeSummary"`
	StepsSummary             map[string]SummaryDetails `json:"stepsSummary"`
	KindSummary              map[string]int64          `json:"kindSummary"`
	StoreSummary             map[string]int64          `json:"storeSummary"`
	DeploymentTypeSummary    map[string]int64          `json:"deploymentTypeSummary"`
	DeploymentsSummary       map[string]SummaryDetails `json:"deploymentsSummary"`
	ArtifactsSummary         map[string]SummaryDetails `json:"artifactsSummary"`
	CloudProviderTypeSummary map[string]int64          `json:"cloudProviderTypeSummary"`
	Expressions              []string                  `json:"expressions"`
}

type BaseEntityDetail struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type ProjectCSV struct {
	AppName           string `json:"appName"`
	ProjectName       string `json:"projectName"`
	ProjectIdentifier string `json:"projectIdentifier"`
	OrgIdentifier     string `json:"orgIdentifier"`
}

type SecretStore struct {
	ApiKey    string `json:"apiKey"`
	AccountId string `json:"accountId"`
	BaseURL   string `json:"baseUrl"`
}
type SecretSpec struct {
	Value                   string `json:"value,omitempty"`
	SecretManagerIdentifier string `json:"secretManagerIdentifier"`
	ValueType               string `json:"valueType"`
}
type Secret struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Identifier  string `json:"identifier"`
	Description string `json:"description,omitempty"`
	Tags        struct {
	} `json:"tags,omitempty"`
	OrgIdentifier     string      `json:"orgIdentifier,omitempty"`
	ProjectIdentifier string      `json:"projectIdentifier,omitempty"`
	Spec              interface{} `json:"spec"`
}

type HarnessSecret struct {
	Secret `json:"secret"`
}

type HarnessService struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name"`
	ProjectIdentifier string `json:"projectIdentifier,omitempty"`
	OrgIdentifier     string `json:"orgIdentifier,omitempty"`
	Description       string `json:"description,omitempty"`
	Tags              struct {
	} `json:"tags,omitempty"`
	Yaml string `json:"yaml"`
}

type GitOpsCluster struct {
	Identifier        string `json:"identifier"`
	ProjectIdentifier string `json:"projectIdentifier"`
	OrgIdentifier     string `json:"orgIdentifier"`
	Description       string `json:"description"`
	Type              string `json:"type,omitempty"`
	Cluster           `json:"cluster"`
}

type GitOpsClusterWithUpdatedFields struct {
	Cluster       `json:"cluster"`
	UpdatedFields []string `json:"updatedFields"`
}

type Cluster struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	Config `json:"config"`
}

type Config struct {
	ClusterConnectionType string `json:"clusterConnectionType"`
}

type Repo struct {
	Repo           string `json:"repo"`
	Type           string `json:"type"`
	Name           string `json:"name"`
	Project        string `json:"project,omitempty"`
	ConnectionType string `json:"connectionType"`
	GithubType     string `json:"githubType,omitempty"`
	InheritedCreds bool   `json:"inheritedCreds,omitempty"`
}
type HarnessRepository struct {
	Repo `json:"repo"`
}

type UpdateMask struct {
	Paths []string `json:"Paths"`
}

type RepoWithUpdateMask struct {
	Repo       `json:"repo"`
	UpdateMask struct {
		Paths []string `json:"paths"`
	} `json:"updateMask"`
}
type HarnessEnvironment struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name"`
	ProjectIdentifier string `json:"projectIdentifier,omitempty"`
	OrgIdentifier     string `json:"orgIdentifier,omitempty"`
	Description       string `json:"description,omitempty"`
	Tags              struct {
	} `json:"tags,omitempty"`
	Color string `json:"color,omitempty"`
	Type  string `json:"type,omitempty"`
	Yaml  string `json:"yaml"`
}

type HarnessInfra struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name"`
	ProjectIdentifier string `json:"projectIdentifier,omitempty"`
	OrgIdentifier     string `json:"orgIdentifier,omitempty"`
	Description       string `json:"description,omitempty"`
	Tags              struct {
	} `json:"tags,omitempty"`
	Yaml string `json:"yaml"`
}

type HarnessPipeline struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name"`
	ProjectIdentifier string `json:"projectIdentifier,omitempty"`
	OrgIdentifier     string `json:"orgIdentifier,omitempty"`
	Description       string `json:"description,omitempty"`
	Tags              struct {
	} `json:"tags,omitempty"`
	//Branch         string    `json:"branch,omitempty"`
	RepoIdentifier string    `json:"repoIdentifier,omitempty"`
	RootFolder     string    `json:"rootFolder,omitempty"`
	FilePath       string    `json:"filePath,omitempty"`
	BaseBranch     string    `json:"baseBranch,omitempty"`
	ConnectorRef   string    `json:"connectorRef,omitempty"`
	RepoName       string    `json:"repoName,omitempty"`
	StoreType      StoreType `json:"storeType"`
	Yaml           string    `json:"yaml"`
}

const (
	SecretText string = "SecretText"
	SecretFile        = "SecretFile"
	SSHKey            = "SSHKey"
	WinRM             = "WinRmCredentials"
)

const (
	SShSecretType string = "SSH"
)

const (
	NTLM string = "NTLM"
)

type SSHSecretType struct {
	Auth SecretAuth `json:"auth"`
	Port int        `json:"port"`
}

type WinRMSecretType struct {
	Auth       WinRMSecretAuth `json:"auth"`
	Port       int             `json:"port"`
	Parameters []string        `json:"parameters,omitempty"`
}

type SecretAuth struct {
	Type string        `json:"type"`
	Spec SSHSecretSpec `json:"spec"`
}

type WinRMSecretAuth struct {
	Type string          `json:"type"`
	Spec WinRMSecretSpec `json:"spec"`
}

type WinRMSecretSpec struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	Domain         string `json:"domain"`
	UseNoProfile   bool   `json:"useNoProfile"`
	UseSSL         bool   `json:"useSSL"`
	SkipCertChecks bool   `json:"skipCertChecks"`
}

type SSHSecretSpec struct {
	CredentialType string           `json:"credentialType"`
	Spec           SShSecretSubSpec `json:"spec"`
}

type SShSecretSubSpec struct {
	UserName string `json:"userName"`
	Key      string `json:"key"`
}

type SSHWINRMSecretData struct {
	Username string
	Key      string
	Port     int
	Password string
	Domain   string
	AuthType string
}
