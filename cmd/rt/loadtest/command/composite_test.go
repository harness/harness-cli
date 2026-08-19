package command

import (
	"strings"
	"testing"
)

// The hyphens that every load test identity uses are exactly what a pipeline
// identifier refuses, so the natural name to reach for is the one that fails.
// Unchecked it comes back as an HTTP 500 quoting a pipeline rule, with nothing
// tying it to the flag.
func TestCompositeIdentifierRejectsHyphensWithARewrite(t *testing.T) {
	err := validateCompositeIdentifier("hc-qa-composite-1")
	if err == nil {
		t.Fatal("expected a hyphenated identifier to be refused")
	}
	if !strings.Contains(err.Error(), "--identifier") {
		t.Errorf("error does not name the flag: %v", err)
	}
	// Naming the fix matters more than naming the rule: the caller wants to
	// retry, not to learn pipeline-service grammar.
	if !strings.Contains(err.Error(), `"hc_qa_composite_1"`) {
		t.Errorf("got %q, want the corrected identifier offered", err)
	}
}

// A leading digit cannot be fixed by swapping separators, so offering a
// rewrite that fails the same way would send the caller round the loop twice.
func TestCompositeIdentifierOffersNoRewriteItCannotStandBehind(t *testing.T) {
	err := validateCompositeIdentifier("1-checkout")
	if err == nil {
		t.Fatal("expected an identifier starting with a digit to be refused")
	}
	if strings.Contains(err.Error(), "try ") {
		t.Errorf("suggested a rewrite that is still invalid: %v", err)
	}
}

func TestCompositeIdentifierAcceptsPipelineNaming(t *testing.T) {
	for _, identifier := range []string{"checkout_resilience", "Checkout1", "a", "a$b_C9"} {
		if err := validateCompositeIdentifier(identifier); err != nil {
			t.Errorf("%q is valid pipeline naming, got %v", identifier, err)
		}
	}
}

// The cap is reported as a length rather than as the character rule, since a
// name that is merely too long is otherwise well formed.
func TestCompositeIdentifierReportsTheLengthCapAsItself(t *testing.T) {
	err := validateCompositeIdentifier("a" + strings.Repeat("b", compositeIdentifierMaxLen))
	if err == nil {
		t.Fatal("expected an over-long identifier to be refused")
	}
	if !strings.Contains(err.Error(), "128") {
		t.Errorf("got %q, want the cap named", err)
	}
}
