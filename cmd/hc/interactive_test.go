package main

import (
	"testing"
)

func TestStripGlobalFlags_NoFlags(t *testing.T) {
	args := []string{"registry", "list"}
	rest := stripGlobalFlags(args)
	if len(rest) != 2 || rest[0] != "registry" || rest[1] != "list" {
		t.Errorf("expected [registry list], got %v", rest)
	}
}

func TestStripGlobalFlags_BoolFlags(t *testing.T) {
	args := []string{"--verbose", "--no-color", "registry", "list"}
	rest := stripGlobalFlags(args)
	if len(rest) != 2 || rest[0] != "registry" || rest[1] != "list" {
		t.Errorf("expected [registry list], got %v", rest)
	}
}

func TestStripGlobalFlags_ValueFlags(t *testing.T) {
	args := []string{"--api-url", "https://example.com", "--account", "abc", "registry", "list"}
	rest := stripGlobalFlags(args)
	if len(rest) != 2 || rest[0] != "registry" || rest[1] != "list" {
		t.Errorf("expected [registry list], got %v", rest)
	}
}

func TestStripGlobalFlags_EqualsSyntax(t *testing.T) {
	args := []string{"--api-url=https://example.com", "registry", "list"}
	rest := stripGlobalFlags(args)
	if len(rest) != 2 || rest[0] != "registry" || rest[1] != "list" {
		t.Errorf("expected [registry list], got %v", rest)
	}
}

func TestStripGlobalFlags_OnlyFlags(t *testing.T) {
	args := []string{"--verbose", "-i", "--no-color"}
	rest := stripGlobalFlags(args)
	if len(rest) != 0 {
		t.Errorf("expected empty slice, got %v", rest)
	}
}

func TestStripGlobalFlags_Empty(t *testing.T) {
	rest := stripGlobalFlags(nil)
	if len(rest) != 0 {
		t.Errorf("expected empty slice, got %v", rest)
	}
}

func TestStripGlobalFlags_ShortVerbose(t *testing.T) {
	args := []string{"-v", "auth", "login"}
	rest := stripGlobalFlags(args)
	if len(rest) != 2 || rest[0] != "auth" || rest[1] != "login" {
		t.Errorf("expected [auth login], got %v", rest)
	}
}

func TestStripGlobalFlags_MixedFlagsAndSubcommand(t *testing.T) {
	args := []string{"--format", "json", "--json", "-v", "artifact", "list", "--page-size", "20"}
	rest := stripGlobalFlags(args)
	// --page-size is NOT a global flag, so it stays along with "20"
	if len(rest) != 4 {
		t.Errorf("expected 4 remaining args, got %v", rest)
	}
}
