package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func ConfirmInput(question string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: question,
	}
	_ = survey.AskOne(prompt, &confirm)
	return confirm
}

func TextInput(question string) string {
	var text = ""
	prompt := &survey.Input{
		Message: question,
	}
	err := survey.AskOne(prompt, &text, survey.WithValidator(survey.Required))
	if err != nil {
		log.Error(err.Error())
		os.Exit(0)
	}
	return text
}

func GetUrlWithQueryParams(environment string, service string, endpoint string, queryParams map[string]string) string {
	params := ""
	totalItems := len(queryParams)
	currentIndex := 0
	for k, v := range queryParams {
		currentIndex++
		if v != "" {
			if currentIndex == totalItems {
				params = params + k + "=" + v
			} else {
				params = params + k + "=" + v + "&"
			}
		}
	}
	// remove trailing & char
	lastChar := params[len(params)-1]
	if lastChar == '&' {
		params = strings.TrimSuffix(params, string('&'))
	}

	return fmt.Sprintf("%s/%s?%s", service, endpoint, params)
}

func printJson(v any) {
	marsheld, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		log.Fatalf("Marshaling error: %s", err)
	}
	fmt.Println("Data: ", string(marsheld))
}

func writeToFile(text string, filename string, overwrite bool) {
	exactFilePath := getUserHomePath() + "/" + filename
	var permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if overwrite {
		permissions = os.O_APPEND | os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(exactFilePath, permissions, 0644)
	if overwrite {
		f.WriteString("")
	}
	f.WriteString(text)
	if err != nil {
		log.Fatal(err)
	}

	f.Close()
}

func readFromFile(filepath string) (s string, r []byte) {
	var _fileContents = ""

	file, _ := os.OpenFile(filepath, os.O_RDONLY, 0644)
	defer file.Close()

	byteValue, readError := io.ReadAll(file)
	if readError != nil {
		//log.Println("Error reading from file:", fileError)
	}
	_fileContents = string(byteValue)

	return _fileContents, byteValue
}

func saveCredentials(c *cli.Context) (err error) {
	baseURL := c.String("base-url")
	if baseURL == "" {
		baseURL = cliCdRequestData.BaseUrl
	}
	if cliCdRequestData.BaseUrl == "" {
		baseURL = HARNESS_PROD_URL
	}
	authCredentials := SecretStore{
		ApiKey:    cliCdRequestData.AuthToken,
		AccountId: cliCdRequestData.Account,
		BaseURL:   baseURL,
	}
	jsonObj, err := json.MarshalIndent(authCredentials, "", "  ")
	if err != nil {
		fmt.Println("Error creating secrets json:", err)
		return
	}

	writeToFile(string(jsonObj), SECRETS_STORE_PATH, true)
	println(getColoredText("Login successfully done. Yay!", color.FgGreen))
	return nil
}

func hydrateCredsFromPersistence(params ...interface{}) {
	c := params[0].(*cli.Context)
	var hydrateOnlyURL = false

	if len(params) > 1 {
		if value, ok := params[1].(bool); ok {
			hydrateOnlyURL = value
		}
	}
	if cliCdRequestData.AuthToken != "" && cliCdRequestData.Account != "" && !hydrateOnlyURL {
		return
	}

	exactFilePath := getUserHomePath() + "/" + SECRETS_STORE_PATH
	credsJson, err := os.ReadFile(exactFilePath)
	if err != nil {
		fmt.Println("Error reading creds file:", err)
		return
	}
	var secretstore SecretStore
	err = json.Unmarshal(credsJson, &secretstore)
	if err != nil {
		fmt.Println("Error:", err)
		Login(c)
		return
	}
	if hydrateOnlyURL {
		baseURL := c.String("base-url")
		if baseURL == "" {
			cliCdRequestData.BaseUrl = secretstore.BaseURL
		} else {
			cliCdRequestData.BaseUrl = baseURL
		}
	} else {
		cliCdRequestData.AuthToken = secretstore.ApiKey
		cliCdRequestData.Account = secretstore.AccountId
		cliCdRequestData.BaseUrl = secretstore.BaseURL

	}
	return
}

func getNGBaseURL(c *cli.Context) string {
	baseURL := c.String("base-url")
	if baseURL == "" {
		if cliCdRequestData.BaseUrl == "" {
			baseURL = HARNESS_PROD_URL
		} else {
			baseURL = cliCdRequestData.BaseUrl
		}
	}

	baseURL = strings.TrimRight(baseURL, "/") //remove trailing slash
	baseURL = baseURL + NG_BASE_URL
	return baseURL
}

func getBaseUrl(c *cli.Context, serviceUrl string) string {
	baseURL := c.String("base-url")

	if baseURL == "" {
		if cliCdRequestData.BaseUrl == "" {
			baseURL = HARNESS_PROD_URL
		} else {
			baseURL = cliCdRequestData.BaseUrl
		}
	}

	baseURL = strings.TrimRight(baseURL, "/") //remove trailing slash
	baseURL = baseURL + serviceUrl
	return baseURL
}

func getJsonFromYaml(content string) map[string]interface{} {
	requestBody := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(content), requestBody)
	if err != nil {
		return nil
	}
	return requestBody
}
func GetNestedValue(data map[string]interface{}, keys ...string) interface{} {
	if len(keys) == 0 {
		return nil
	}

	value, ok := data[keys[0]]
	if !ok {
		return nil
	}

	for _, key := range keys[1:] {
		nested, ok := value.(map[string]interface{})
		if !ok {
			return nil
		}
		value, ok = nested[key]
		if !ok {
			return nil
		}
	}

	return value
}
func getUserHomePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Failed to get user's home directory:", err)
		return ""
	}
	return homeDir
}
func valueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		return fmt.Sprintf("%f", v)
	case float64:
		return fmt.Sprintf("%f", v)
	default:
		return ""
	}
}

func getColoredText(text string, textColor color.Attribute) string {
	colored := color.New(textColor).SprintFunc()
	return colored(text)
}

func getEntity(baseUrl string, reqURL string, projectIdentifier string, orgIdentifier string, extraParams map[string]string) bool {
	queryParams := map[string]string{
		"accountIdentifier": cliCdRequestData.Account,
		"routingId":         cliCdRequestData.Account,
		"projectIdentifier": projectIdentifier,
		"orgIdentifier":     orgIdentifier,
	}
	queryParams = mergeMaps(queryParams, extraParams)
	urlWithQueryParams := GetUrlWithQueryParams("", baseUrl, reqURL, queryParams)
	_, fetchEntityError := Get(urlWithQueryParams, cliCdRequestData.AuthToken)
	if fetchEntityError != nil {
		return false
	} else {
		return true
	}
}

func mergeMaps(map1 map[string]string, map2 map[string]string) map[string]string {
	mergedMap := func() map[string]string {
		result := make(map[string]string)
		for k, v := range map1 {
			result[k] = v
		}
		for k, v := range map2 {
			result[k] = v
		}
		return result
	}()
	return mergedMap
}

// replaces placeholder values in the given yaml content
func replacePlaceholderValues(haystack string, needle string, value string) string {
	return strings.ReplaceAll(haystack, needle, value)
}

func fetchCloudType(str string) string {
	gcpRegexPattern := `type:\s+GoogleCloudFunctions`
	awsRegexPattern := `type:\s+AwsLambda`

	if isGcpMatch, err1 := regexp.MatchString(gcpRegexPattern, str); err1 == nil && isGcpMatch {
		return GCP
	} else if isAwsMatch, err2 := regexp.MatchString(awsRegexPattern, str); err2 == nil && isAwsMatch {
		return AWS
	}

	return ""
}
