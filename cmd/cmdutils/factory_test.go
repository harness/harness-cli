package cmdutils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harness/harness-cli/config"
	"github.com/harness/harness-cli/internal/api/ar_pkg"
	"github.com/harness/harness-cli/util/common/progress"
)

// mockReporter implements progress.Reporter for testing
type mockReporter struct {
	steps   []string
	errors  []string
	started bool
	ended   bool
}

func (m *mockReporter) Start(message string) {
	m.started = true
}

func (m *mockReporter) End() {
	m.ended = true
}

func (m *mockReporter) Step(msg string) {
	m.steps = append(m.steps, msg)
}

func (m *mockReporter) Success(msg string) {
	m.steps = append(m.steps, msg)
}

func (m *mockReporter) Error(msg string) {
	m.errors = append(m.errors, msg)
}

func (m *mockReporter) Warn(msg string) {
	m.errors = append(m.errors, msg)
}

var _ progress.Reporter = (*mockReporter)(nil)

// setupTestServer creates a test HTTP server and configures global config
func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	// Save original config
	origAPIBaseURL := config.Global.APIBaseURL
	origPkgURL := config.Global.Registry.PkgURL
	origAccountID := config.Global.AccountID
	origAuthToken := config.Global.AuthToken

	// Set test config
	config.Global.APIBaseURL = srv.URL
	config.Global.Registry.PkgURL = srv.URL
	config.Global.AccountID = "test-account"
	config.Global.AuthToken = "pat.test-account.aaa.bbb"

	// Restore on cleanup
	t.Cleanup(func() {
		config.Global.APIBaseURL = origAPIBaseURL
		config.Global.Registry.PkgURL = origPkgURL
		config.Global.AccountID = origAccountID
		config.Global.AuthToken = origAuthToken
	})

	return srv
}

func TestNewFactory(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	if factory == nil {
		t.Fatal("NewFactory() returned nil")
	}

	if factory.RegistryHttpClient == nil {
		t.Error("RegistryHttpClient function is nil")
	}

	if factory.RegistryV2HttpClient == nil {
		t.Error("RegistryV2HttpClient function is nil")
	}

	if factory.RegistryV3HttpClient == nil {
		t.Error("RegistryV3HttpClient function is nil")
	}

	if factory.PkgHttpClient == nil {
		t.Error("PkgHttpClient function is nil")
	}
}

func TestFactory_RegistryHttpClient(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	client := factory.RegistryHttpClient()

	if client == nil {
		t.Fatal("RegistryHttpClient() returned nil client")
	}
}

func TestFactory_RegistryV2HttpClient(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	client := factory.RegistryV2HttpClient()

	if client == nil {
		t.Fatal("RegistryV2HttpClient() returned nil client")
	}
}

func TestFactory_RegistryV3HttpClient(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	client := factory.RegistryV3HttpClient()

	if client == nil {
		t.Fatal("RegistryV3HttpClient() returned nil client")
	}
}

func TestFactory_PkgHttpClient(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	client := factory.PkgHttpClient()

	if client == nil {
		t.Fatal("PkgHttpClient() returned nil client")
	}
}

func TestFactory_NewRegistryV3HttpClientWithURL(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	client, err := factory.NewRegistryV3HttpClientWithURL(srv.URL)

	if err != nil {
		t.Fatalf("NewRegistryV3HttpClientWithURL() returned error: %v", err)
	}

	if client == nil {
		t.Fatal("NewRegistryV3HttpClientWithURL() returned nil client")
	}
}

func TestFactory_NewRegistryV3HttpClientWithURL_CustomURL(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	customURL := srv.URL + "/custom/path"
	client, err := factory.NewRegistryV3HttpClientWithURL(customURL)

	if err != nil {
		t.Fatalf("NewRegistryV3HttpClientWithURL() with custom URL returned error: %v", err)
	}

	if client == nil {
		t.Fatal("NewRegistryV3HttpClientWithURL() with custom URL returned nil client")
	}
}

func TestFactory_PkgHttpClientWithProgress(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(1024)
	filename := "test.zip"

	client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client")
	}
}

func TestFactory_PkgHttpClientWithProgress_ZeroFileSize(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(0)
	filename := "empty.txt"

	client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client for zero file size")
	}
}

func TestFactory_CustomPkgHttpClient(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	customClientCalled := false
	factory := &Factory{
		PkgHttpClient: func() *ar_pkg.ClientWithResponses {
			customClientCalled = true
			client, _ := ar_pkg.NewClientWithResponses(srv.URL)
			return client
		},
	}

	client := factory.PkgHttpClient()

	if client == nil {
		t.Fatal("Custom PkgHttpClient() returned nil")
	}

	if !customClientCalled {
		t.Error("Custom PkgHttpClient function was not called")
	}
}

func TestFactory_MultipleClientCreations(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Create multiple clients to ensure factory can be reused
	client1 := factory.PkgHttpClient()
	client2 := factory.PkgHttpClient()

	if client1 == nil {
		t.Error("First PkgHttpClient() call returned nil")
	}

	if client2 == nil {
		t.Error("Second PkgHttpClient() call returned nil")
	}

	// Note: These will be different instances since the function creates new clients each time
	if client1 == client2 {
		t.Log("Note: Factory creates new client instances on each call (expected behavior)")
	}
}

func TestFactory_AllClientsCanBeCreatedSimultaneously(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Test that all client types can be created without conflicts
	regClient := factory.RegistryHttpClient()
	regV2Client := factory.RegistryV2HttpClient()
	regV3Client := factory.RegistryV3HttpClient()
	pkgClient := factory.PkgHttpClient()

	if regClient == nil {
		t.Error("RegistryHttpClient is nil")
	}
	if regV2Client == nil {
		t.Error("RegistryV2HttpClient is nil")
	}
	if regV3Client == nil {
		t.Error("RegistryV3HttpClient is nil")
	}
	if pkgClient == nil {
		t.Error("PkgHttpClient is nil")
	}
}

func TestFactory_PkgHttpClientWithProgress_LargeFile(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(1024 * 1024 * 100) // 100MB
	filename := "large-file.tar.gz"

	client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client for large file")
	}
}

func TestFactory_PkgHttpClientWithProgress_DifferentExtensions(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(1024)

	testCases := []string{
		"package.tar.gz",
		"package.zip",
		"package.jar",
		"package.war",
		"package.nupkg",
		"package.tgz",
		"package.conda",
		"package.tar.bz2",
	}

	for _, filename := range testCases {
		t.Run(filename, func(t *testing.T) {
			client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)
			if client == nil {
				t.Errorf("PkgHttpClientWithProgress() returned nil for %s", filename)
			}
		})
	}
}

func TestFactory_NewRegistryV3HttpClientWithURL_DifferentURLs(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	testCases := []struct {
		name string
		url  string
	}{
		{"base URL", srv.URL},
		{"with path", srv.URL + "/api/v3"},
		{"with query", srv.URL + "?param=value"},
		{"with port", srv.URL},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := factory.NewRegistryV3HttpClientWithURL(tc.url)
			if err != nil {
				t.Errorf("NewRegistryV3HttpClientWithURL(%s) returned error: %v", tc.url, err)
			}
			if client == nil {
				t.Errorf("NewRegistryV3HttpClientWithURL(%s) returned nil client", tc.url)
			}
		})
	}
}

func TestFactory_EmptyFactory(t *testing.T) {
	// Test that an empty factory struct can be created
	factory := &Factory{}

	if factory.RegistryHttpClient != nil {
		t.Error("Empty factory should have nil RegistryHttpClient")
	}
	if factory.RegistryV2HttpClient != nil {
		t.Error("Empty factory should have nil RegistryV2HttpClient")
	}
	if factory.RegistryV3HttpClient != nil {
		t.Error("Empty factory should have nil RegistryV3HttpClient")
	}
	if factory.PkgHttpClient != nil {
		t.Error("Empty factory should have nil PkgHttpClient")
	}
}

func TestFactory_PartialFactory(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// Test factory with only some clients defined
	factory := &Factory{
		PkgHttpClient: func() *ar_pkg.ClientWithResponses {
			client, _ := ar_pkg.NewClientWithResponses(srv.URL)
			return client
		},
	}

	if factory.PkgHttpClient == nil {
		t.Error("PkgHttpClient should not be nil")
	}
	if factory.RegistryHttpClient != nil {
		t.Error("RegistryHttpClient should be nil in partial factory")
	}

	client := factory.PkgHttpClient()
	if client == nil {
		t.Error("PkgHttpClient() should return a client")
	}
}

func TestFactory_ConcurrentClientCreation(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Test concurrent access to factory methods
	done := make(chan bool, 4)

	go func() {
		client := factory.RegistryHttpClient()
		if client == nil {
			t.Error("Concurrent RegistryHttpClient() returned nil")
		}
		done <- true
	}()

	go func() {
		client := factory.RegistryV2HttpClient()
		if client == nil {
			t.Error("Concurrent RegistryV2HttpClient() returned nil")
		}
		done <- true
	}()

	go func() {
		client := factory.RegistryV3HttpClient()
		if client == nil {
			t.Error("Concurrent RegistryV3HttpClient() returned nil")
		}
		done <- true
	}()

	go func() {
		client := factory.PkgHttpClient()
		if client == nil {
			t.Error("Concurrent PkgHttpClient() returned nil")
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestFactory_PkgHttpClientWithProgress_NilReporter(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	fileSize := int64(1024)
	filename := "test.zip"

	// Test with nil reporter (should still work)
	client := factory.PkgHttpClientWithProgress(nil, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client with nil reporter")
	}
}

func TestFactory_PkgHttpClientWithProgress_EmptyFilename(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(1024)
	filename := ""

	client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client with empty filename")
	}
}

func TestFactory_PkgHttpClientWithProgress_NegativeFileSize(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()
	reporter := &mockReporter{}
	fileSize := int64(-1)
	filename := "test.zip"

	client := factory.PkgHttpClientWithProgress(reporter, fileSize, filename)

	if client == nil {
		t.Fatal("PkgHttpClientWithProgress() returned nil client with negative file size")
	}
}

func TestFactory_MultipleNewFactoryCalls(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	// Create multiple factories and ensure they're independent
	factory1 := NewFactory()
	factory2 := NewFactory()

	if factory1 == factory2 {
		t.Error("NewFactory() should return different instances")
	}

	client1 := factory1.PkgHttpClient()
	client2 := factory2.PkgHttpClient()

	if client1 == nil {
		t.Error("Factory1 PkgHttpClient is nil")
	}
	if client2 == nil {
		t.Error("Factory2 PkgHttpClient is nil")
	}
}

func TestFactory_RegistryHttpClient_MultipleCalls(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Call multiple times to ensure it works consistently
	for i := 0; i < 5; i++ {
		client := factory.RegistryHttpClient()
		if client == nil {
			t.Errorf("RegistryHttpClient() call %d returned nil", i+1)
		}
	}
}

func TestFactory_RegistryV2HttpClient_MultipleCalls(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Call multiple times to ensure it works consistently
	for i := 0; i < 5; i++ {
		client := factory.RegistryV2HttpClient()
		if client == nil {
			t.Errorf("RegistryV2HttpClient() call %d returned nil", i+1)
		}
	}
}

func TestFactory_RegistryV3HttpClient_MultipleCalls(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()

	factory := NewFactory()

	// Call multiple times to ensure it works consistently
	for i := 0; i < 5; i++ {
		client := factory.RegistryV3HttpClient()
		if client == nil {
			t.Errorf("RegistryV3HttpClient() call %d returned nil", i+1)
		}
	}
}
