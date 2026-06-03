package utils

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFileChecksums(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Hello, World! This is a test file for checksum computation."

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	checksums, err := ComputeFileChecksums(testFile)
	if err != nil {
		t.Fatalf("ComputeFileChecksums failed: %v", err)
	}

	// Verify that all checksums are computed and non-empty
	if checksums.MD5 == "" {
		t.Error("MD5 checksum is empty")
	}
	if checksums.SHA1 == "" {
		t.Error("SHA1 checksum is empty")
	}
	if checksums.SHA256 == "" {
		t.Error("SHA256 checksum is empty")
	}
	if checksums.SHA512 == "" {
		t.Error("SHA512 checksum is empty")
	}

	// Verify that checksums have expected lengths (hex encoded)
	if len(checksums.MD5) != 32 {
		t.Errorf("MD5 checksum has incorrect length: got %d, want 32", len(checksums.MD5))
	}
	if len(checksums.SHA1) != 40 {
		t.Errorf("SHA1 checksum has incorrect length: got %d, want 40", len(checksums.SHA1))
	}
	if len(checksums.SHA256) != 64 {
		t.Errorf("SHA256 checksum has incorrect length: got %d, want 64", len(checksums.SHA256))
	}
	if len(checksums.SHA512) != 128 {
		t.Errorf("SHA512 checksum has incorrect length: got %d, want 128", len(checksums.SHA512))
	}
}

func TestComputeFileChecksums_NonExistentFile(t *testing.T) {
	_, err := ComputeFileChecksums("/non/existent/file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestComputeFileChecksums_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Deterministic test content"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	checksums1, err1 := ComputeFileChecksums(testFile)
	if err1 != nil {
		t.Fatalf("First ComputeFileChecksums failed: %v", err1)
	}

	checksums2, err2 := ComputeFileChecksums(testFile)
	if err2 != nil {
		t.Fatalf("Second ComputeFileChecksums failed: %v", err2)
	}

	// Verify that checksums are deterministic
	if checksums1.MD5 != checksums2.MD5 {
		t.Error("MD5 checksums are not deterministic")
	}
	if checksums1.SHA1 != checksums2.SHA1 {
		t.Error("SHA1 checksums are not deterministic")
	}
	if checksums1.SHA256 != checksums2.SHA256 {
		t.Error("SHA256 checksums are not deterministic")
	}
	if checksums1.SHA512 != checksums2.SHA512 {
		t.Error("SHA512 checksums are not deterministic")
	}
}

func TestSetChecksumHeaders(t *testing.T) {
	headers := http.Header{}
	checksums := FileChecksums{
		MD5:    "d41d8cd98f00b204e9800998ecf8427e",
		SHA1:   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA512: "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
	}

	SetChecksumHeaders(headers, checksums)

	// Verify all headers are set
	if headers.Get("X-Checksum-Md5") != checksums.MD5 {
		t.Errorf("X-Checksum-Md5 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Md5"), checksums.MD5)
	}
	if headers.Get("X-Checksum-Sha1") != checksums.SHA1 {
		t.Errorf("X-Checksum-Sha1 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Sha1"), checksums.SHA1)
	}
	if headers.Get("X-Checksum-Sha256") != checksums.SHA256 {
		t.Errorf("X-Checksum-Sha256 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Sha256"), checksums.SHA256)
	}
	if headers.Get("X-Checksum-Sha512") != checksums.SHA512 {
		t.Errorf("X-Checksum-Sha512 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Sha512"), checksums.SHA512)
	}
}

func TestSetChecksumHeaders_PartialChecksums(t *testing.T) {
	headers := http.Header{}
	checksums := FileChecksums{
		MD5:    "d41d8cd98f00b204e9800998ecf8427e",
		SHA1:   "",
		SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SHA512: "",
	}

	SetChecksumHeaders(headers, checksums)

	// Verify only non-empty checksums are set
	if headers.Get("X-Checksum-Md5") != checksums.MD5 {
		t.Errorf("X-Checksum-Md5 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Md5"), checksums.MD5)
	}
	if headers.Get("X-Checksum-Sha1") != "" {
		t.Error("X-Checksum-Sha1 header should not be set for empty SHA1")
	}
	if headers.Get("X-Checksum-Sha256") != checksums.SHA256 {
		t.Errorf("X-Checksum-Sha256 header not set correctly: got %s, want %s", headers.Get("X-Checksum-Sha256"), checksums.SHA256)
	}
	if headers.Get("X-Checksum-Sha512") != "" {
		t.Error("X-Checksum-Sha512 header should not be set for empty SHA512")
	}
}

func TestSetChecksumHeaders_EmptyChecksums(t *testing.T) {
	headers := http.Header{}
	checksums := FileChecksums{}

	SetChecksumHeaders(headers, checksums)

	// Verify no headers are set for empty checksums
	if headers.Get("X-Checksum-Md5") != "" {
		t.Error("X-Checksum-Md5 header should not be set for empty MD5")
	}
	if headers.Get("X-Checksum-Sha1") != "" {
		t.Error("X-Checksum-Sha1 header should not be set for empty SHA1")
	}
	if headers.Get("X-Checksum-Sha256") != "" {
		t.Error("X-Checksum-Sha256 header should not be set for empty SHA256")
	}
	if headers.Get("X-Checksum-Sha512") != "" {
		t.Error("X-Checksum-Sha512 header should not be set for empty SHA512")
	}
}

func TestSetChecksumHeaders_OverwritesExisting(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Checksum-Md5", "old-md5-value")
	headers.Set("X-Checksum-Sha256", "old-sha256-value")

	checksums := FileChecksums{
		MD5:    "new-md5-value",
		SHA1:   "new-sha1-value",
		SHA256: "new-sha256-value",
		SHA512: "new-sha512-value",
	}

	SetChecksumHeaders(headers, checksums)

	// Verify headers are overwritten
	if headers.Get("X-Checksum-Md5") != "new-md5-value" {
		t.Error("X-Checksum-Md5 header was not overwritten")
	}
	if headers.Get("X-Checksum-Sha256") != "new-sha256-value" {
		t.Error("X-Checksum-Sha256 header was not overwritten")
	}
}

func TestChecksumHeadersForFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "Test content for ChecksumHeadersForFile"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	headers, checksums, err := ChecksumHeadersForFile(testFile)
	if err != nil {
		t.Fatalf("ChecksumHeadersForFile failed: %v", err)
	}

	// Verify headers are set
	if headers.Get("X-Checksum-Md5") == "" {
		t.Error("X-Checksum-Md5 header not set")
	}
	if headers.Get("X-Checksum-Sha1") == "" {
		t.Error("X-Checksum-Sha1 header not set")
	}
	if headers.Get("X-Checksum-Sha256") == "" {
		t.Error("X-Checksum-Sha256 header not set")
	}
	if headers.Get("X-Checksum-Sha512") == "" {
		t.Error("X-Checksum-Sha512 header not set")
	}

	// Verify checksums struct is populated
	if checksums.MD5 == "" {
		t.Error("MD5 checksum is empty")
	}
	if checksums.SHA1 == "" {
		t.Error("SHA1 checksum is empty")
	}
	if checksums.SHA256 == "" {
		t.Error("SHA256 checksum is empty")
	}
	if checksums.SHA512 == "" {
		t.Error("SHA512 checksum is empty")
	}

	// Verify headers match checksums
	if headers.Get("X-Checksum-Md5") != checksums.MD5 {
		t.Error("X-Checksum-Md5 header does not match MD5 checksum")
	}
	if headers.Get("X-Checksum-Sha1") != checksums.SHA1 {
		t.Error("X-Checksum-Sha1 header does not match SHA1 checksum")
	}
	if headers.Get("X-Checksum-Sha256") != checksums.SHA256 {
		t.Error("X-Checksum-Sha256 header does not match SHA256 checksum")
	}
	if headers.Get("X-Checksum-Sha512") != checksums.SHA512 {
		t.Error("X-Checksum-Sha512 header does not match SHA512 checksum")
	}
}

func TestChecksumHeadersForFile_NonExistentFile(t *testing.T) {
	_, _, err := ChecksumHeadersForFile("/non/existent/file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}
