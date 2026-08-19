package command

import (
	"strings"
	"testing"
)

// A settable path names the tool, but --runtime-input declares the same field
// without it at create time, so both forms have to resolve.
func TestRuntimePathAcceptsEveryFormOfTheSamePath(t *testing.T) {
	const want = ".toolConfig.k6.tunables.targetUsers"
	settable := []string{want}

	for _, typed := range []string{
		".toolConfig.k6.tunables.targetUsers", // as get-variables prints it
		"toolConfig.k6.tunables.targetUsers",  // without the leading dot
		"k6.tunables.targetUsers",             // rooted at the tool
		"tunables.targetUsers",                // the create-time shorthand
		"targetUsers",                         // the field alone
	} {
		got, err := matchRuntimePath("checkout-load", typed, settable)
		if err != nil {
			t.Errorf("%q was not resolved: %v", typed, err)
			continue
		}
		if got != want {
			t.Errorf("%q resolved to %q, want %q", typed, got, want)
		}
	}
}

// A path is matched a segment at a time, so a field is not reached by half of
// its name.
func TestRuntimePathDoesNotMatchAPartialSegment(t *testing.T) {
	settable := []string{".toolConfig.k6.tunables.targetUsers"}

	for _, typed := range []string{"Users", "argetUsers", "tunables.target"} {
		if _, err := matchRuntimePath("checkout-load", typed, settable); err == nil {
			t.Errorf("%q was accepted as a path", typed)
		}
	}
}

// An abbreviation that fits two inputs is a question, not a choice to make on
// the caller's behalf: setting the wrong one would change the run silently.
func TestRuntimePathRefusesAnAmbiguousAbbreviation(t *testing.T) {
	settable := []string{
		".toolConfig.k6.tunables.targetUsers",
		".toolConfig.k6.scenarios.spike.targetUsers",
	}

	_, err := matchRuntimePath("checkout-load", "targetUsers", settable)
	if err == nil {
		t.Fatal("an abbreviation naming two inputs was accepted")
	}
	for _, path := range settable {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("the error does not offer %q: %s", path, err)
		}
	}

	// The full path still resolves, which is what the error tells them to do.
	got, err := matchRuntimePath("checkout-load", settable[1], settable)
	if err != nil || got != settable[1] {
		t.Fatalf("the unambiguous form was not accepted: %q, %v", got, err)
	}
}

// The service answers an unknown path with a 400 once the run is already
// requested. Refusing here costs a read and can say what the test does take.
func TestRuntimePathReportsWhatTheTestAccepts(t *testing.T) {
	settable := []string{".toolConfig.k6.tunables.targetUsers", ".toolConfig.k6.tunables.durationSeconds"}

	_, err := matchRuntimePath("checkout-load", "rampUp", settable)
	if err == nil {
		t.Fatal("an unknown path was accepted")
	}
	for _, path := range settable {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("the error does not list %q: %s", path, err)
		}
	}
}

// A test with nothing authored as "<+input>" cannot take a runtime value at
// all, which is a different mistake from naming the wrong path.
func TestRuntimePathSaysWhenThereAreNoInputsAtAll(t *testing.T) {
	_, err := matchRuntimePath("checkout-load", "targetUsers", nil)
	if err == nil {
		t.Fatal("a runtime value was accepted by a test with no runtime inputs")
	}
	if !strings.Contains(err.Error(), "<+input>") {
		t.Fatalf("the error does not say how to make the field settable: %s", err)
	}
}
