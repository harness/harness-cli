package main

import (
	"bytes"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMainFunction_Help(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"harness", "--help"}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stdout)
	}()
	main()
	assert.Contains(t, buf.String(), "NAME:\n  harness - CLI utility to interact with Harness Platform to manage various Harness modules and its diverse set of resources.")
}

func TestMainFunction_Login(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	os.Args = []string{"harness", "login", "--api-key=testkey", "--account-id=testaccount"}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer func() {
		log.SetOutput(os.Stdout)
	}()
	main()
	assert.Contains(t, buf.String(), "some expected output from login")
}
