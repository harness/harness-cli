package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tmaxmax/go-sse"
)

const (
	streamEndpoint      = "/gateway/log-service/stream"
	blobEndpoint        = "/gateway/log-service/blob"
	accountIDQueryParam = "accountID"
	keyQueryParam       = "key"
	authHeaderKey       = "X-Harness-Token"
)

type LogClient struct {
	client    sse.Client
	endpoint  string
	token     string
	accountID string
}

type Line struct {
	Out   string
	Time  string
	Level string
}

type LogError struct {
	Message string `json:"message"`
}

func (e *LogError) Error() string {
	return e.Message
}

func NewLogClient(endpoint, accountID, token string) *LogClient {
	client := sse.Client{
		Backoff: sse.Backoff{
			MaxRetries: 5,
		},
		HTTPClient: http.DefaultClient,
	}
	return &LogClient{
		endpoint:  endpoint,
		accountID: accountID,
		token:     token,
		client:    client,
	}
}

func (c *LogClient) Tail(ctx context.Context, key string) error {
	url, err := url.Parse(c.endpoint)
	if err != nil {
		return err
	}
	url.Path = streamEndpoint
	query := url.Query()
	query.Set(accountIDQueryParam, c.accountID)
	query.Set(keyQueryParam, key)
	url.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Set(authHeaderKey, c.token)
	conn := c.client.NewConnection(req)
	conn.SubscribeMessages(func(event sse.Event) {
		line, err := formatLogs(event.Data)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(line)
		}
	})
	err = conn.Connect()
	if err != nil {
		return err
	}
	return nil
}

func (c *LogClient) Blob(ctx context.Context, key string) (int, error) {
	url, err := url.Parse(c.endpoint)
	if err != nil {
		return 0, err
	}
	url.Path = blobEndpoint
	query := url.Query()
	query.Set(accountIDQueryParam, c.accountID)
	query.Set(keyQueryParam, key)
	url.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set(authHeaderKey, c.token)
	resp, err := c.client.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 {
		logErr := &LogError{}
		if err := json.Unmarshal(body, logErr); err != nil {
			return 0, err
		}
		return 0, logErr
	}

	lineCount, err := readLines(string(body))
	if err != nil {
		return 0, err
	}
	return lineCount, nil
}

func readLines(lines string) (int, error) {
	lineCount := 0
	scanner := bufio.NewScanner(strings.NewReader(lines))
	for scanner.Scan() {
		lineCount += 1
		line, err := formatLogs(scanner.Text())
		if err != nil {
			return lineCount, err
		}
		fmt.Println(line)
	}
	return lineCount, nil
}

func formatLogs(line string) (string, error) {
	decodedLine := &Line{}
	err := json.Unmarshal([]byte(line), decodedLine)
	if err != nil {
		return "", err
	}

	timestamp, err := time.Parse(time.RFC3339, decodedLine.Time)
	if err != nil {
		fmt.Println(err)
	}

	out := strings.Replace(decodedLine.Out, "\u001b[33;", "", -1)
	formattedTimestamp := timestamp.Format("02/01/2006 15:04:05")
	return fmt.Sprintf(
		"%s %s %s",
		strings.ToUpper(decodedLine.Level),
		formattedTimestamp,
		strings.TrimSpace(out),
	), nil
}
