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

// resetRegistry swaps in a fresh empty registry for the duration of the
// calling test and restores the previous registry (e.g. the init()-populated
// production registry) via t.Cleanup. This keeps the package's tests
// order-independent: Go compiles test files alphabetically, so without a
// restore, handler_test.go's tests would run before matrix_test.go and leave
// `registry` permanently empty.
func resetRegistry(t *testing.T) {
	t.Helper()
	saved := registry
	registry = map[types.ArtifactType]Handler{}
	t.Cleanup(func() {
		registry = saved
	})
}

func TestRegisterGetRoundTrip(t *testing.T) {
	resetRegistry(t)

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
	resetRegistry(t)

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
	resetRegistry(t)

	h := stubHandler{typ: types.ArtifactType("")}

	if err := Register(h); err == nil {
		t.Fatal("expected error registering handler with empty Type(), got nil")
	}
}

func TestRegisterNilHandler(t *testing.T) {
	resetRegistry(t)

	if err := Register(nil); err == nil {
		t.Fatal("expected error registering nil handler, got nil")
	}
}

func TestGetUnregistered(t *testing.T) {
	resetRegistry(t)

	h, err := Get(types.ArtifactType("HANDLER_TEST_MISSING"))
	if h != nil {
		t.Fatalf("expected nil handler for unregistered type, got %v", h)
	}
	if err == nil {
		t.Fatal("expected error for unregistered type, got nil")
	}
}
