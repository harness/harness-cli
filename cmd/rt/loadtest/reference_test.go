package loadtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// These guards live here, not in package command, because the tree is only
// whole once NewCmd has assembled it and a child cannot import its parent.

// commandReference matches "hc rt loadtest <name> [sub]" in prose. The separator
// is [ \t]+ not \s+, so a match cannot span lines and swallow the next one.
var commandReference = regexp.MustCompile(`hc rt loadtest ([a-z][a-z0-9-]*)(?:[ \t]+([a-z][a-z0-9-]*))?`)

// words that follow "hc rt loadtest" in prose without naming a subcommand.
var notACommand = map[string]bool{
	"commands": true,
	"to":       true,
}

// rootCommand is the node every reference in prose is relative to.
func rootCommand() *cobra.Command { return NewCmd() }

// walk visits every command in the tree, including those in sub-groups.
func walk(cmd *cobra.Command, visit func(*cobra.Command)) {
	for _, sub := range cmd.Commands() {
		visit(sub)
		walk(sub, visit)
	}
}

// lookup resolves a command by name or alias among a parent's children.
func lookup(parent *cobra.Command, name string) *cobra.Command {
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
		for _, alias := range sub.Aliases {
			if alias == name {
				return sub
			}
		}
	}
	return nil
}

// TestReferencedCommandsExist stops help text and error messages pointing at a
// command that is not registered. A stale cross-reference compiles perfectly.
func TestReferencedCommandsExist(t *testing.T) {
	root := rootCommand()
	if len(root.Commands()) == 0 {
		t.Fatal("no subcommands registered: the guard would pass vacuously")
	}

	sources, err := filepath.Glob(filepath.Join("command", "*.go"))
	if err != nil {
		t.Fatalf("globbing sources: %v", err)
	}
	// The examples README is checked alongside the code because it lives in
	// this tree and its commands go stale the same way help text does.
	sources = append(sources, "root.go", filepath.Join("examples", "README.md"))

	checked := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		body, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("reading %s: %v", source, err)
		}
		for _, match := range commandReference.FindAllStringSubmatch(string(body), -1) {
			name, sub := match[1], match[2]
			if notACommand[name] {
				continue
			}
			checked++

			command := lookup(root, name)
			if command == nil {
				t.Errorf("%s refers to %q, which is not a registered command", source, match[0])
				continue
			}
			// A sub-group named on its own is as broken as a misspelt verb.
			if !command.HasSubCommands() {
				continue
			}
			if sub == "" {
				t.Errorf("%s refers to %q, but %q is a command group and needs a subcommand",
					source, match[0], name)
				continue
			}
			if lookup(command, sub) == nil {
				t.Errorf("%s refers to %q, which %q does not provide", source, match[0], name)
			}
		}
	}

	if checked == 0 {
		t.Fatal("found no command references to check: the regex has probably drifted")
	}
}

// exampleFlag matches a "--flag" token in an Example block.
var exampleFlag = regexp.MustCompile(`--([a-z0-9][a-z0-9-]*)`)

// globalFlags is a copy of the hc root's persistent flags, which live in
// package main and cannot be imported. TestGlobalFlagListIsCurrent keeps it honest.
var globalFlags = map[string]bool{
	"account":        true,
	"api-url":        true,
	"format":         true,
	"no-progress":    true,
	"org":            true,
	"profile":        true,
	"profile-output": true,
	"project":        true,
	"timeout":        true,
	"token":          true,
	"verbose":        true,
}

// TestExampleFlagsExist catches an Example using a flag its command never
// registers, which nothing else would: it is only a string constant.
func TestExampleFlagsExist(t *testing.T) {
	checked := 0

	walk(rootCommand(), func(sub *cobra.Command) {
		if sub.Example == "" {
			return
		}

		declared := map[string]bool{}
		sub.Flags().VisitAll(func(f *pflag.Flag) { declared[f.Name] = true })
		sub.InheritedFlags().VisitAll(func(f *pflag.Flag) { declared[f.Name] = true })

		for _, match := range exampleFlag.FindAllStringSubmatch(sub.Example, -1) {
			name := match[1]
			if globalFlags[name] {
				continue
			}
			checked++
			if !declared[name] {
				t.Errorf("the example for %q uses --%s, which the command does not register",
					sub.CommandPath(), name)
			}
		}
	})

	if checked == 0 {
		t.Fatal("found no example flags to check: the regex has probably drifted")
	}
}

// TestGlobalFlagListIsCurrent fails on a dead exemption — one whose flag every
// using command declares locally — that TestExampleFlagsExist could be checking.
func TestGlobalFlagListIsCurrent(t *testing.T) {
	for name := range globalFlags {
		used, declaredEverywhere := false, true
		walk(rootCommand(), func(sub *cobra.Command) {
			if sub.Example == "" || !strings.Contains(sub.Example, "--"+name) {
				return
			}
			used = true
			if sub.Flags().Lookup(name) == nil {
				declaredEverywhere = false
			}
		})
		if used && declaredEverywhere {
			t.Errorf("--%s is in globalFlags but every example using it belongs to a command that declares it locally; drop the exemption so it is checked", name)
		}
	}
}
