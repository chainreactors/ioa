package client

import (
	"testing"

	"github.com/chainreactors/ioa/protocols"
)

type testIdentity struct{ binding protocols.IdentityBinding }

func (i testIdentity) IOABinding() protocols.IdentityBinding { return i.binding }

func TestClientIdentityBindingIsInMemory(t *testing.T) {
	c, err := NewClient("https://secret@example.test/ioa", "")
	if err != nil {
		t.Fatal(err)
	}
	if !c.NodeRef().Valid() || c.Bound() {
		t.Fatalf("new identity = %#v bound=%v", c.NodeRef(), c.Bound())
	}
	binding := protocols.IdentityBinding{Namespace: "example.system", Subject: "worker-1"}
	if err := c.Bind(testIdentity{binding: binding}); err != nil {
		t.Fatal(err)
	}
	got := c.identityBindings()
	if len(got) != 1 || got[0].Namespace != binding.Namespace || got[0].Subject != binding.Subject {
		t.Fatalf("bindings = %#v", got)
	}
}
