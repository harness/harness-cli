package utils

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"os"
)

// FileChecksums holds hex-encoded digests of a file.
type FileChecksums struct {
	MD5    string
	SHA1   string
	SHA256 string
	SHA512 string
}

// ComputeFileChecksums computes MD5, SHA-1, SHA-256 and SHA-512 of the file at
// path in a single read pass and returns them hex-encoded.
func ComputeFileChecksums(path string) (FileChecksums, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileChecksums{}, err
	}
	defer f.Close()

	md5h := md5.New()
	sha1h := sha1.New()
	sha256h := sha256.New()
	sha512h := sha512.New()

	mw := io.MultiWriter(md5h, sha1h, sha256h, sha512h)
	if _, err := io.Copy(mw, f); err != nil {
		return FileChecksums{}, err
	}

	hexSum := func(h hash.Hash) string { return hex.EncodeToString(h.Sum(nil)) }
	return FileChecksums{
		MD5:    hexSum(md5h),
		SHA1:   hexSum(sha1h),
		SHA256: hexSum(sha256h),
		SHA512: hexSum(sha512h),
	}, nil
}

// SetChecksumHeaders sets X-Checksum-{Md5,Sha1,Sha256,Sha512} headers on h
// for any non-empty digest in c.
func SetChecksumHeaders(h http.Header, c FileChecksums) {
	//fmt.Print(c.MD5 + "\n" + string(c.SHA1) + "\n" + string(c.SHA256) + "\n" + string(c.SHA512))
	if c.MD5 != "" {
		h.Set("X-Checksum-Md5", c.MD5)
	}
	if c.SHA1 != "" {
		h.Set("X-Checksum-Sha1", c.SHA1)
	}
	if c.SHA256 != "" {
		h.Set("X-Checksum-Sha256", c.SHA256)
	}
	if c.SHA512 != "" {
		h.Set("X-Checksum-Sha512", c.SHA512)
	}
}

// ChecksumHeadersForFile is a convenience helper that computes file checksums
// and returns an http.Header pre-populated with X-Checksum-* entries.
func ChecksumHeadersForFile(path string) (http.Header, FileChecksums, error) {
	c, err := ComputeFileChecksums(path)
	if err != nil {
		return nil, FileChecksums{}, err
	}
	h := http.Header{}
	SetChecksumHeaders(h, c)
	return h, c, nil
}
