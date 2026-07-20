package server

import (
	"context"
	"net/http/httptest"
	"testing"

	ioaclient "github.com/chainreactors/ioa/client"
	"github.com/chainreactors/ioa/protocols"
)

type testExternalIdentity struct{ ref protocols.NodeRef }

func (i testExternalIdentity) IOABinding() protocols.IdentityBinding {
	return protocols.IdentityBinding{Namespace: "test.web", Subject: i.ref.URI()}
}

func TestNodeIdentityBindings(t *testing.T) {
	ctx := context.Background()
	service := NewService(NewMemoryStore(), "access")
	id := "0123456789abcdef0123456789abcdef"
	binding := protocols.IdentityBinding{
		Namespace: "example.system",
		Subject:   "worker-1",
		Claims:    map[string]any{"profile": map[string]any{"role": "scanner"}},
	}
	resp, err := service.AuthRegister(ctx, protocols.AuthRegister{
		ID: id, Name: "worker", AccessKey: "access", Identities: []protocols.IdentityBinding{binding},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != id {
		t.Fatalf("node ID = %q", resp.ID)
	}

	resolved, err := service.ResolveNodeIdentity(ctx, binding.Namespace, binding.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != id || len(resolved.Identities) != 1 {
		t.Fatalf("resolved node = %#v", resolved)
	}

	otherID := "fedcba9876543210fedcba9876543210"
	rebound, err := service.AuthRegister(ctx, protocols.AuthRegister{
		ID: otherID, Name: "other", AccessKey: "access", Identities: []protocols.IdentityBinding{binding},
	})
	if err != nil || rebound.ID != id {
		t.Fatalf("binding did not resolve existing node: response=%#v error=%v", rebound, err)
	}

	updated, err := service.UpsertNodeIdentity(ctx, id, id, protocols.IdentityBinding{
		Namespace: binding.Namespace, Subject: binding.Subject, Claims: map[string]any{"version": float64(2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := updated.Identities[0].Claims["version"]; got != float64(2) {
		t.Fatalf("updated claims = %#v", updated.Identities[0].Claims)
	}

	updated, err = service.DeleteNodeIdentity(ctx, id, id, binding.Namespace, binding.Subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Identities) != 0 {
		t.Fatalf("identities after delete = %#v", updated.Identities)
	}
}

func TestLegacyAuthRegisterStillResolvesByName(t *testing.T) {
	ctx := context.Background()
	service := NewService(NewMemoryStore(), "access")
	first, err := service.AuthRegister(ctx, protocols.AuthRegister{Name: "legacy", AccessKey: "access"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.AuthRegister(ctx, protocols.AuthRegister{Name: "legacy", AccessKey: "access"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("legacy registration changed ID: %s != %s", first.ID, second.ID)
	}
}

func TestSDKBindingResolvesNodeAcrossClients(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(NewHandler(NewService(NewMemoryStore(), "")))
	defer srv.Close()
	identity := testExternalIdentity{ref: protocols.NodeRef{ID: "worker", Authority: "https://web.example"}}

	first, err := ioaclient.NewClient(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Bind(identity); err != nil {
		t.Fatal(err)
	}
	if err := first.EnsureRegistered(ctx, "worker", "", nil); err != nil {
		t.Fatal(err)
	}

	second, err := ioaclient.NewClient(srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Bind(identity); err != nil {
		t.Fatal(err)
	}
	if err := second.EnsureRegistered(ctx, "worker", "", nil); err != nil {
		t.Fatal(err)
	}
	if second.NodeID() != first.NodeID() {
		t.Fatalf("binding resolved different nodes: %s != %s", second.NodeID(), first.NodeID())
	}
}
