package rt

import (
	"strings"
	"testing"
)

// "hc rt" is a group with nothing to run of its own, so it has to list what is
// under it. A group that neither runs nor lists is a dead end at the terminal.
func TestTheRTGroupOffersLoadTesting(t *testing.T) {
	root := GetRootCmd()

	if root.Use != "rt" {
		t.Errorf("use = %q, want the group to be reached as \"hc rt\"", root.Use)
	}
	if root.Runnable() {
		t.Error("the group is runnable, so \"hc rt\" would do something rather than list its commands")
	}

	loadtest, _, err := root.Find([]string{"loadtest"})
	if err != nil {
		t.Fatalf("finding the loadtest group: %v", err)
	}
	if loadtest.Name() != "loadtest" {
		t.Fatalf("\"hc rt loadtest\" resolved to %q", loadtest.Name())
	}
}

// Resiliency Testing is not a term anyone will guess from "rt", and chaos
// commands are expected here too. Both belong in the help rather than in a
// commit message.
func TestTheRTGroupSaysWhatItStandsFor(t *testing.T) {
	root := GetRootCmd()

	for _, want := range []string{"Resiliency Testing", "Chaos"} {
		if !strings.Contains(root.Long+root.Short, want) {
			t.Errorf("the help does not mention %q:\n%s\n%s", want, root.Short, root.Long)
		}
	}
}
