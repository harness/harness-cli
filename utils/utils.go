package utils

import (
	"encoding/json"
	"fmt"
	"harness/client"
	"harness/defaults"
	. "harness/shared"
	. "harness/types"
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
        if len(queryParams) > 0 {
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
        } else {
            return fmt.Sprintf("%s/%s", service, endpoint)
        }
}

func PrintJson(v any) {
	marsheld, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		log.Fatalf("Marshaling error: %s", err)
	}
	fmt.Println("Data: ", string(marsheld))
}

func WriteToFile(text string, filename string, overwrite bool) {
	exactFilePath := GetUserHomePath() + "/" + filename
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

func ReadFromFile(filepath string) (s string, r []byte) {
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

func SaveCredentials(c *cli.Context, showWelcome bool) (err error) {
	baseURL := c.String("base-url")
	if baseURL == "" {
		baseURL = CliCdRequestData.BaseUrl
	}
	if CliCdRequestData.BaseUrl == "" {
		baseURL = defaults.HARNESS_PROD_URL
	}
	authCredentials := SecretStore{
		ApiKey:    CliCdRequestData.AuthToken,
		AccountId: CliCdRequestData.Account,
		BaseURL:   baseURL,
		UserId:    CliCdRequestData.UserId,
	}
	jsonObj, err := json.MarshalIndent(authCredentials, "", "  ")
	if err != nil {
		fmt.Println("Error creating secrets json:", err)
		return
	}

	WriteToFile(string(jsonObj), defaults.SECRETS_STORE_PATH, true)
	if showWelcome {
		println(GetColoredText("Login successfully done. Yay!", color.FgGreen))
	}
	return nil
}

func GetNGBaseURL(c *cli.Context) string {
	baseURL := c.String("base-url")
	if baseURL == "" {
		if CliCdRequestData.BaseUrl == "" {
			baseURL = defaults.HARNESS_PROD_URL
		} else {
			baseURL = CliCdRequestData.BaseUrl
		}
	}

	baseURL = strings.TrimRight(baseURL, "/") //remove trailing slash
	baseURL = baseURL + defaults.NG_BASE_URL
	return baseURL
}

func GetBaseUrl(c *cli.Context, serviceUrl string) string {
	baseURL := c.String("base-url")

	if baseURL == "" {
		if CliCdRequestData.BaseUrl == "" {
			baseURL = defaults.HARNESS_PROD_URL
		} else {
			baseURL = CliCdRequestData.BaseUrl
		}
	}

	baseURL = strings.TrimRight(baseURL, "/") //remove trailing slash
	baseURL = baseURL + serviceUrl
	return baseURL
}

func GetJsonFromYaml(content string) map[string]interface{} {
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
func GetUserHomePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Failed to get user's home directory:", err)
		return ""
	}
	return homeDir
}
func ValueToString(value interface{}) string {
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

func GetColoredText(text string, textColor color.Attribute) string {
	colored := color.New(textColor).SprintFunc()
	return colored(text)
}

func GetEntity(baseUrl string, reqURL string, projectIdentifier string, orgIdentifier string, extraParams map[string]string) bool {
	queryParams := map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
		"routingId":         CliCdRequestData.Account,
		"projectIdentifier": projectIdentifier,
		"orgIdentifier":     orgIdentifier,
	}
	queryParams = MergeMaps(queryParams, extraParams)
	urlWithQueryParams := GetUrlWithQueryParams("", baseUrl, reqURL, queryParams)
	_, fetchEntityError := client.Get(urlWithQueryParams, CliCdRequestData.AuthToken)
	if fetchEntityError != nil {
		return false
	} else {
		return true
	}
}

func MergeMaps(map1 map[string]string, map2 map[string]string) map[string]string {
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
func ReplacePlaceholderValues(haystack string, needle string, value string) string {
	return strings.ReplaceAll(haystack, needle, value)
}

func FetchCloudType(str string) string {
	gcpRegexPattern := `type:\s+GoogleCloudFunctions`
	awsRegexPattern := `type:\s+AwsLambda`

	if isGcpMatch, err1 := regexp.MatchString(gcpRegexPattern, str); err1 == nil && isGcpMatch {
		return defaults.GCP
	} else if isAwsMatch, err2 := regexp.MatchString(awsRegexPattern, str); err2 == nil && isAwsMatch {
		return defaults.AWS
	}

	return ""
}

func PromptAccountDetails(ctx *cli.Context) bool {
	promptConfirm := false

	if len(CliCdRequestData.Account) == 0 {
		promptConfirm = true
		CliCdRequestData.Account = TextInput("Account that you wish to login to:")
	}

	if len(CliCdRequestData.AuthToken) == 0 {
		promptConfirm = true
		CliCdRequestData.AuthToken = TextInput("Provide your api-key:")
	}
	return promptConfirm
}

func GetAccountDetails(ctx *cli.Context) error {
	// Getting the account details
	var baseURL = GetNGBaseURL(ctx)
	accountsEndpoint := defaults.ACCOUNTS_ENDPOINT + CliCdRequestData.Account
	url := GetUrlWithQueryParams("", baseURL, accountsEndpoint, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})
	resp, err := client.Get(url, CliCdRequestData.AuthToken)
	if err != nil {
		println(GetColoredText("Could not log in: Did you provide correct credentials?", color.FgRed))
		fmt.Printf("Response code: %s \n", resp.Code)
		return err
	}
	return nil
}

func GetUserDetails(ctx *cli.Context) error {
	var baseURL = GetNGBaseURL(ctx)
	url := GetUrlWithQueryParams("", baseURL, defaults.USER_INFO_ENDPOINT, map[string]string{
		"accountIdentifier": CliCdRequestData.Account,
	})
	resp, err := client.Get(url, CliCdRequestData.AuthToken)
	if err != nil {
		println(GetColoredText("Could not log in: Did you provide correct credentials?", color.FgRed))
		fmt.Printf("Response code: %s \n", resp.Code)
		return err
	}
	dataJSON, err := json.Marshal(resp.Data)
	if err != nil {
		fmt.Println("Error marshalling data:", err)
		return err
	}
	var currentUserInfo UserInfo
	err = json.Unmarshal(dataJSON, &currentUserInfo)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return err
	}
	CliCdRequestData.UserId = currentUserInfo.Email
	return nil
}

func GetTypeFromYAML(str string) (connectorType string) {
	pattern := `type:\s+(\w+)`
	expression := regexp.MustCompile(pattern)
	if expression.MatchString(str) {
		// Find the first match in the YAML content
		match := expression.FindStringSubmatch(str)

		// Extract the value (abc) from the match
		if len(match) >= 2 {
			value := match[1]
			connectorType = value
		}
	}
	return
}
