package httpclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/harness/harness-cli/util/common/progress"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReporter struct {
	steps  []string
	errors []string
}

func (m *mockReporter) Start(message string) {
	fmt.Printf("  ▶ %s...\n", message)
}

func (m *mockReporter) End() {

}

func (m *mockReporter) Step(msg string) {
	m.steps = append(m.steps, msg)
}

func (m *mockReporter) Success(string) {}

func (m *mockReporter) Error(msg string) {
	m.errors = append(m.errors, msg)
}

func (m *mockReporter) Warn(string) {}

var _ progress.Reporter = (*mockReporter)(nil)

type testReadCloser struct {
	io.Reader
	closed bool
}

func (t *testReadCloser) Close() error {
	t.closed = true
	return nil
}

func TestNewRetryClientWithoutProgress(t *testing.T) {
	client := NewRetryClientWithoutProgress()

	require.NotNil(t, client)
	assert.IsType(t, &http.Client{}, client)
}

func TestNewRetryClientWithProgress(t *testing.T) {
	reporter := &mockReporter{}

	client := NewRetryClientWithProgress(
		reporter,
		1024,
		"test.txt",
	)

	require.NotNil(t, client)
	assert.IsType(t, &http.Client{}, client)
}

func TestRequestLogHookRetryMessage(t *testing.T) {
	reporter := &mockReporter{}

	hook := requestLogHook(
		reporter,
		3,
		0,
		"",
	)

	req, err := http.NewRequest(
		http.MethodPut,
		"http://example.com",
		nil,
	)
	require.NoError(t, err)

	hook(nil, req, 1)

	require.Len(t, reporter.steps, 1)
	assert.Contains(t, reporter.steps[0], "attempt 1/3")
}

func TestRequestLogHookFirstAttemptNoMessage(t *testing.T) {
	reporter := &mockReporter{}

	hook := requestLogHook(
		reporter,
		3,
		0,
		"",
	)

	req, err := http.NewRequest(
		http.MethodPut,
		"http://example.com",
		nil,
	)
	require.NoError(t, err)

	hook(nil, req, 0)

	assert.Empty(t, reporter.steps)
}

func TestRequestLogHookWrapsBody(t *testing.T) {
	reporter := &mockReporter{}

	body := io.NopCloser(
		bytes.NewBufferString("hello"),
	)

	req, err := http.NewRequest(
		http.MethodPut,
		"http://example.com",
		body,
	)
	require.NoError(t, err)

	hook := requestLogHook(
		reporter,
		3,
		10,
		"file.txt",
	)

	hook(nil, req, 0)

	_, ok := req.Body.(*progressReadCloser)
	assert.True(t, ok)
}

func TestRequestLogHookDoesNotWrapWhenFileSizeZero(t *testing.T) {
	reporter := &mockReporter{}

	body := io.NopCloser(
		bytes.NewBufferString("hello"),
	)

	req, err := http.NewRequest(
		http.MethodPut,
		"http://example.com",
		body,
	)
	require.NoError(t, err)

	originalBody := req.Body

	hook := requestLogHook(
		reporter,
		3,
		0,
		"file.txt",
	)

	hook(nil, req, 0)

	//assert.Same(t, originalBody, req.Body)
	assert.True(t, originalBody == req.Body)
}

func TestResponseLogHookLogsFailure(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHook(reporter)

	resp := &http.Response{
		StatusCode: 502,
	}

	hook(nil, resp)

	require.Len(t, reporter.errors, 1)
	assert.Contains(t, reporter.errors[0], "502")
}

func TestResponseLogHookIgnoresSuccess(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHook(reporter)

	resp := &http.Response{
		StatusCode: 200,
	}

	hook(nil, resp)

	assert.Empty(t, reporter.errors)
}

func TestResponseLogHookNilResponse(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHook(reporter)

	hook(nil, nil)

	assert.Empty(t, reporter.errors)
}

func TestProgressReadCloserRead(t *testing.T) {
	rc := &testReadCloser{
		Reader: bytes.NewBufferString("hello world"),
	}

	prc := &progressReadCloser{
		reader: rc,
		bar:    nil,
	}

	buf := make([]byte, 5)

	n, err := prc.Read(buf)

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(buf))
}

func TestProgressReadCloserReadEOF(t *testing.T) {
	rc := &testReadCloser{
		Reader: bytes.NewBuffer(nil),
	}

	prc := &progressReadCloser{
		reader: rc,
		bar:    nil,
	}

	buf := make([]byte, 5)

	n, err := prc.Read(buf)

	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestProgressReadCloserClose(t *testing.T) {
	rc := &testReadCloser{
		Reader: bytes.NewBufferString("data"),
	}

	prc := &progressReadCloser{
		reader: rc,
		bar:    nil,
	}

	err := prc.Close()

	require.NoError(t, err)
	assert.True(t, rc.closed)
}

func TestRequestLogHookNilBody(t *testing.T) {
	reporter := &mockReporter{}

	req, err := http.NewRequest(
		http.MethodPut,
		"http://example.com",
		nil,
	)
	require.NoError(t, err)

	hook := requestLogHook(
		reporter,
		3,
		100,
		"file.txt",
	)

	hook(nil, req, 0)

	assert.Nil(t, req.Body)
}

func TestResponseLogHookSuccessStatus(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHook(reporter)

	resp := &http.Response{
		StatusCode: 200,
	}

	hook(nil, resp)

	assert.Empty(t, reporter.errors)
}
func TestResponseLogHookWithoutProgressSuccess200(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHookWithoutProgress(reporter)

	resp := &http.Response{
		StatusCode: 200,
	}

	hook(nil, resp)

	// Success responses don't log anything
	assert.Empty(t, reporter.steps)
	assert.Empty(t, reporter.errors)
}
func TestResponseLogHookWithoutProgressSuccess201(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHookWithoutProgress(reporter)

	resp := &http.Response{
		StatusCode: 201,
	}

	hook(nil, resp)

	// Success responses don't log anything
	assert.Empty(t, reporter.steps)
	assert.Empty(t, reporter.errors)
}
func TestResponseLogHookWithoutProgressFailure500(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHookWithoutProgress(reporter)

	resp := &http.Response{
		StatusCode: 500,
	}

	hook(nil, resp)

	require.Len(t, reporter.errors, 1)
	assert.Contains(t, reporter.errors[0], "Request failed")
	assert.Contains(t, reporter.errors[0], "500")
}
func TestResponseLogHookWithoutProgressNilResponse(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHookWithoutProgress(reporter)

	hook(nil, nil)

	assert.Empty(t, reporter.steps)
}
func TestResponseLogHookWithoutProgressFailure404(t *testing.T) {
	reporter := &mockReporter{}

	hook := responseLogHookWithoutProgress(reporter)

	resp := &http.Response{
		StatusCode: 404,
	}

	hook(nil, resp)

	require.Len(t, reporter.errors, 1)
	assert.Contains(t, reporter.errors[0], "Request failed")
	assert.Contains(t, reporter.errors[0], "404")
}
