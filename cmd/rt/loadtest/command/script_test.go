package command

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harness/harness-cli/cmd/rt/loadtest/api"
)

// aScriptRevision is a stored script as the service returns one. The content is
// base64 because that is how it travels, and the tests assert on the decoded
// form, since decoding it is the command's job.
func aScriptRevision(number int, contents string) map[string]any {
	return map[string]any{
		"identity":         fmt.Sprintf("rev-%03d", number),
		"loadTestIdentity": "checkout-load",
		"revisionNumber":   number,
		"scriptContent":    base64.StdEncoding.EncodeToString([]byte(contents)),
		"description":      fmt.Sprintf("Revision %d", number),
		"createdAt":        "2026-08-01T09:00:00Z",
		"createdBy":        "qa@harness.io",
	}
}

func TestScriptGetWritesTheDecodedScriptToStdout(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): aScriptRevision(3, "export default function () {}"),
	})

	out := expectSuccess(t, NewScriptCmd(), "get", "checkout-load")

	// Straight to stdout with nothing around it, so it can be redirected to a
	// file or piped into the tool that runs it.
	if out.stdout != "export default function () {}" {
		t.Errorf("stdout = %q, want the decoded script and nothing else", out.stdout)
	}
}

func TestScriptGetSavesToAFileWhenAsked(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): aScriptRevision(3, "export default function () {}"),
	})

	destination := filepath.Join(t.TempDir(), "checkout.js")
	out := expectSuccess(t, NewScriptCmd(), "get", "checkout-load", "--output-file", destination)

	saved, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading what the command saved: %v", err)
	}
	if string(saved) != "export default function () {}" {
		t.Errorf("saved %q, want the decoded script", saved)
	}
	// The confirmation goes to stderr so it cannot corrupt a redirect.
	mustContain(t, "script get confirmation", out.stderr, destination, "bytes")
	if out.stdout != "" {
		t.Errorf("stdout = %q, want it empty when the script went to a file", out.stdout)
	}
}

// A zip workspace is binary. Printing it would fill the terminal with noise and
// look like the command had failed, so it is refused with the way out named.
func TestScriptGetRefusesToPrintAZipWorkspace(t *testing.T) {
	bundle := aScriptRevision(4, "PK\x03\x04binary")
	bundle["isBundle"] = true
	bundle["bundleMainFile"] = "checkout.jmx"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): bundle,
	})

	message := expectFailure(t, NewScriptCmd(), "get", "checkout-load")

	mustContain(t, "bundle refusal", message, "workspace", "--output-file", "checkout.jmx")
}

func TestScriptGetSavesAZipWorkspaceWithoutComplaint(t *testing.T) {
	bundle := aScriptRevision(4, "PK\x03\x04binary")
	bundle["isBundle"] = true

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): bundle,
	})

	destination := filepath.Join(t.TempDir(), "workspace.zip")
	expectSuccess(t, NewScriptCmd(), "get", "checkout-load", "--output-file", destination)

	saved, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("reading the saved workspace: %v", err)
	}
	if string(saved) != "PK\x03\x04binary" {
		t.Errorf("saved %q, want the archive bytes unaltered", saved)
	}
}

// The refusal names the plan to extract when the archive declares one. A
// workspace uploaded without that field still has to be described, or the
// message would read "workspace ()" and offer nothing.
func TestAZipWithoutANamedPlanIsStillDescribedAsOne(t *testing.T) {
	named := bundleKind(&api.ScriptRevision{BundleMainFile: "checkout.jmx"})
	if !strings.Contains(named, "checkout.jmx") || !strings.HasPrefix(named, "zip") {
		t.Errorf("bundleKind = %q, want it to name the main plan", named)
	}
	if got := bundleKind(&api.ScriptRevision{}); got != "zip" {
		t.Errorf("bundleKind = %q, want a bare zip described as one", got)
	}
}

func TestScriptGetReportsAScriptItCannotDecode(t *testing.T) {
	broken := aScriptRevision(3, "")
	broken["scriptContent"] = "not base64 at all!!"

	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): broken,
	})

	message := expectFailure(t, NewScriptCmd(), "get", "checkout-load")

	mustContain(t, "decode error", message, "decoding script")
}

func TestScriptGetReportsAnUnwritablePath(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): aScriptRevision(3, "body"),
	})

	inaccessible := filepath.Join(t.TempDir(), "no-such-directory", "out.js")
	message := expectFailure(t, NewScriptCmd(), "get", "checkout-load", "--output-file", inaccessible)

	mustContain(t, "write error", message, "writing", inaccessible)
}

func TestScriptListRevisionsPrintsNewestFirst(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script/revisions"): []any{
			aScriptRevision(3, "third"),
			aScriptRevision(2, "second"),
			aScriptRevision(1, "first"),
		},
	})

	out := expectSuccess(t, NewScriptCmd(), "list-revisions", "checkout-load")

	mustContain(t, "revision listing", out.stdout, "rev-003", "rev-002", "rev-001")
}

// A revision number is what the listing shows, but the endpoint only takes an
// identity, so a number costs a lookup. Both forms have to work.
func TestScriptGetRevisionResolvesANumberThroughTheListing(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script/revisions"): []any{
			aScriptRevision(3, "third"),
			aScriptRevision(2, "second"),
		},
		GET("/load-tests/checkout-load/script/revisions/rev-002"): aScriptRevision(2, "second"),
	})

	out := expectSuccess(t, NewScriptCmd(), "get-revision", "checkout-load", "2")

	if out.stdout != "second" {
		t.Errorf("stdout = %q, want the body of revision 2", out.stdout)
	}
	if len(stub.requests()) != 2 {
		t.Errorf("made %v, want a listing lookup then the fetch", summarise(stub.requests()))
	}
}

func TestScriptGetRevisionTakesAnIdentityWithoutALookup(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script/revisions/rev-002"): aScriptRevision(2, "second"),
	})

	expectSuccess(t, NewScriptCmd(), "get-revision", "checkout-load", "rev-002")

	// An identity is already what the endpoint wants, so the listing is skipped.
	if got := stub.only().Path; !strings.HasSuffix(got, "/revisions/rev-002") {
		t.Errorf("got %s, want the revision fetched directly", got)
	}
}

func TestScriptGetRevisionNamesTheRevisionsThatDoExist(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script/revisions"): []any{
			aScriptRevision(3, "third"),
			aScriptRevision(1, "first"),
		},
	})

	message := expectFailure(t, NewScriptCmd(), "get-revision", "checkout-load", "9")

	mustContain(t, "missing revision error", message, "no script revision 9", "3", "1")
}

func TestScriptGetRevisionReportsATestWithNoRevisions(t *testing.T) {
	serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script/revisions"): []any{},
	})

	message := expectFailure(t, NewScriptCmd(), "get-revision", "checkout-load", "1")

	mustContain(t, "empty revisions error", message, "no script revisions")
}

func TestScriptUpdateUploadsTheFileAsBase64(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		PUT("/load-tests/checkout-load/script"): aScriptRevision(4, "export default function () {}"),
	})

	path := scriptFile(t, "checkout-v2.js", "export default function () {}")
	out := expectSuccess(t, NewScriptCmd(), "update", "checkout-load",
		"--script", path, "--description", "Add the payment step")

	var body api.UpdateScriptRequest
	stub.only().decode(t, &body)

	decoded, err := base64.StdEncoding.DecodeString(body.ScriptContent)
	if err != nil {
		t.Fatalf("the uploaded content is not base64: %v", err)
	}
	if string(decoded) != "export default function () {}" {
		t.Errorf("uploaded %q, want the file contents", decoded)
	}
	if body.Description != "Add the payment step" {
		t.Errorf("description = %q, want the one passed", body.Description)
	}
	mustContain(t, "upload confirmation", out.stderr, "revision 4")
}

func TestScriptUpdateInsistsOnAScript(t *testing.T) {
	serveLTM(t, map[call]any{})

	message := expectFailure(t, NewScriptCmd(), "update", "checkout-load")

	mustContain(t, "missing script error", message, "--script is required")
}

func TestScriptUpdateReportsAScriptThatIsNotThere(t *testing.T) {
	serveLTM(t, map[call]any{})

	missing := filepath.Join(t.TempDir(), "absent.js")
	message := expectFailure(t, NewScriptCmd(), "update", "checkout-load", "--script", missing)

	mustContain(t, "missing file error", message, "absent.js")
}

// The upload is the whole point of the command, so a failure has to surface
// rather than be reported as a revision that was never created.
func TestScriptUpdateSurfacesARejectedUpload(t *testing.T) {
	serveLTM(t, map[call]any{
		PUT("/load-tests/checkout-load/script"): http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"script exceeds the size limit"}`, http.StatusBadRequest)
		}),
	})

	path := scriptFile(t, "big.js", "export default function () {}")
	message := expectFailure(t, NewScriptCmd(), "update", "checkout-load", "--script", path)

	mustContain(t, "rejected upload", message, "size limit")
}

func TestScriptCommandsCarryTheResolvedScope(t *testing.T) {
	stub := serveLTM(t, map[call]any{
		GET("/load-tests/checkout-load/script"): aScriptRevision(1, "body"),
	})

	expectSuccess(t, NewScriptCmd(), "get", "checkout-load")

	query := stub.only().Query
	for key, want := range map[string]string{
		"accountIdentifier":      "acct1",
		"organizationIdentifier": "default",
		"projectIdentifier":      "perf",
	} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}
