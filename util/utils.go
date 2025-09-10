package util

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Safe[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// SetupLogging configures logging to file and stdout
func SetupLogging(logDir string, logLevel string) error {
	// Create log directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	logFile := filepath.Join(logDir, fmt.Sprintf("ar-migration_%s.log", timestamp))

	// Open the log file for writing
	logFileHandle, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Configure multi-writer to write to both stdout and the log file
	multiWriter := io.MultiWriter(os.Stdout, logFileHandle)
	log.SetOutput(multiWriter)

	// Set log flags
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Implement log level filtering if needed based on logLevel parameter
	// This is a simple implementation - a more sophisticated logging library
	// could be used in a real implementation

	fmt.Printf("Logs will be written to: %s\n", logFile)
	return nil
}

// ValidateMapping validates a ar mapping
func ValidateMapping(sourceRegistry, destinationRegistry string) error {
	if sourceRegistry == "" {
		return fmt.Errorf("source ar cannot be empty")
	}

	if destinationRegistry == "" {
		return fmt.Errorf("destination ar cannot be empty")
	}

	// Validate destination ar format
	// Format could be: "ar", "org/ar", or "org/project/ar"
	parts := strings.Split(destinationRegistry, "/")
	if len(parts) > 3 {
		return fmt.Errorf("invalid destination ar format: %s (must be at most 3 levels deep)", destinationRegistry)
	}

	return nil
}

// ParseDestinationRegistryPath parses a destination ar path into its components
func ParseDestinationRegistryPath(registryPath string) (string, string, string) {
	parts := strings.Split(registryPath, "/")

	switch len(parts) {
	case 1:
		// "ar" format - account level
		return parts[0], "", ""
	case 2:
		// "org/ar" format - org level
		return parts[1], parts[0], ""
	case 3:
		// "org/project/ar" format - project level
		return parts[2], parts[0], parts[1]
	default:
		// Invalid format, return empty strings
		return "", "", ""
	}
}

// ProgressBar returns a string representing a progress bar
func ProgressBar(current, total int, width int) string {
	if total == 0 {
		return "[--------------------]"
	}

	percentage := float64(current) / float64(total)
	completed := int(percentage * float64(width))

	bar := "["
	for i := 0; i < width; i++ {
		if i < completed {
			bar += "="
		} else {
			bar += "-"
		}
	}
	bar += "]"

	return fmt.Sprintf("%s %.1f%%", bar, percentage*100)
}

// GetPkgUrl returns the URL for the packages
func GetPkgUrl(url string) string {
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	return url
}
