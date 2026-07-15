package handler

import (
	"testing"

	"github.com/harness/harness-cli/module/ar/migrate/types"
)

type stubHandler struct {
	base
	typ types.ArtifactType
}

func (s stubHandler) Type() types.ArtifactType { return s.typ }

func resetRegistry() {
	registry = map[types.ArtifactType]Handler{}
}

func TestRegisterGetRoundTrip(t *testing.T) {
	resetRegistry()

	typ := types.ArtifactType("HANDLER_TEST_OK")
	h := stubHandler{typ: typ}

	if err := Register(h); err != nil {
		t.Fatalf("Register returned unexpected error: %v", err)
	}

	got, err := Get(typ)
	if err != nil {
		t.Fatalf("Get returned unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil handler")
	}
	if got.Type() != typ {
		t.Fatalf("expected Type() %q, got %q", typ, got.Type())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	resetRegistry()

	typ := types.ArtifactType("HANDLER_TEST_DUP")
	h := stubHandler{typ: typ}

	if err := Register(h); err != nil {
		t.Fatalf("first Register returned unexpected error: %v", err)
	}

	if err := Register(h); err == nil {
		t.Fatal("expected error registering duplicate handler, got nil")
	}
}

func TestRegisterEmptyType(t *testing.T) {
	resetRegistry()

	h := stubHandler{typ: types.ArtifactType("")}

	if err := Register(h); err == nil {
		t.Fatal("expected error registering handler with empty Type(), got nil")
	}
}

func TestRegisterNilHandler(t *testing.T) {
	resetRegistry()

	if err := Register(nil); err == nil {
		t.Fatal("expected error registering nil handler, got nil")
	}
}

func TestGetUnregistered(t *testing.T) {
	resetRegistry()

	h, err := Get(types.ArtifactType("HANDLER_TEST_MISSING"))
	if h != nil {
		t.Fatalf("expected nil handler for unregistered type, got %v", h)
	}
	if err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}
