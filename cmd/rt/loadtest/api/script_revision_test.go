package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The endpoint matches the path segment against a generated id, but the listing
// prints the ordinal under REVISION, so that number has to resolve too.
func TestGetScriptRevisionTakesTheNumberTheListingPrints(t *testing.T) {
	const wanted = "b8b7521f-2f31-4a16-86a2-9b824e40ffe3"

	var fetched string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/script/revisions") {
			_ = json.NewEncoder(w).Encode([]ScriptRevision{
				{Identity: wanted, RevisionNumber: 2},
				{Identity: "01804989-5f83-47e2-abde-bbcde0ee521e", RevisionNumber: 1},
			})
			return
		}
		fetched = r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_ = json.NewEncoder(w).Encode(ScriptRevision{Identity: fetched, RevisionNumber: 2, ScriptContent: "aGk="})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	revision, err := client.GetScriptRevision(context.Background(), "checkout-load", "2")
	if err != nil {
		t.Fatalf("GetScriptRevision by number: %v", err)
	}
	if fetched != wanted {
		t.Fatalf("fetched revision %q, want the identity of revision 2, %q", fetched, wanted)
	}
	if revision.ScriptContent == "" {
		t.Fatal("returned an empty revision")
	}
}

// An identity is what the endpoint takes, so it goes straight through: a
// script pinned to a fixed version must not pay for a listing.
func TestGetScriptRevisionPassesAnIdentityStraightThrough(t *testing.T) {
	const identity = "b8b7521f-2f31-4a16-86a2-9b824e40ffe3"

	var listings int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/script/revisions") {
			listings++
			_ = json.NewEncoder(w).Encode([]ScriptRevision{})
			return
		}
		_ = json.NewEncoder(w).Encode(ScriptRevision{Identity: identity, RevisionNumber: 2})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	revision, err := client.GetScriptRevision(context.Background(), "checkout-load", identity)
	if err != nil {
		t.Fatalf("GetScriptRevision by identity: %v", err)
	}
	if revision.Identity != identity {
		t.Fatalf("returned revision %q, want %q", revision.Identity, identity)
	}
	if listings != 0 {
		t.Fatalf("listed the revisions %d times to resolve an identity", listings)
	}
}

// The service can only say "not found: 4". Having read the listing already,
// the client can say which numbers there are.
func TestGetScriptRevisionNamesTheRevisionsThatExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/script/revisions") {
			t.Errorf("fetched a revision that could not have been resolved: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]ScriptRevision{
			{Identity: "b8b7521f", RevisionNumber: 2},
			{Identity: "01804989", RevisionNumber: 1},
		})
	}))
	defer server.Close()

	client := NewClient(Config{Server: server.URL})
	_, err := client.GetScriptRevision(context.Background(), "checkout-load", "4")
	if err == nil {
		t.Fatal("a revision number that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "1") || !strings.Contains(err.Error(), "2") {
		t.Fatalf("the error does not say which revisions exist: %v", err)
	}
}
