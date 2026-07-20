package protocols

import "testing"

func TestNodeRefCanonicalURI(t *testing.T) {
	authority, err := CanonicalAuthority("https://token@example.test/ioa/?x=1#fragment")
	if err != nil {
		t.Fatal(err)
	}
	if authority != "https://example.test/ioa" {
		t.Fatalf("authority = %q", authority)
	}
	ref := NodeRef{ID: "0123456789abcdef0123456789abcdef", Authority: authority}
	if got := ref.URI(); got != "https://example.test/ioa/nodes/0123456789abcdef0123456789abcdef" {
		t.Fatalf("URI = %q", got)
	}
}

func TestNormalizeIdentityBindingsPreservesNestedClaims(t *testing.T) {
	bindings, err := NormalizeIdentityBindings([]IdentityBinding{{
		Namespace: " example.system ",
		Subject:   " node-1 ",
		Claims:    map[string]any{"nested": map[string]any{"roles": []any{"scan", "chat"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if bindings[0].Namespace != "example.system" || bindings[0].Subject != "node-1" {
		t.Fatalf("binding = %#v", bindings[0])
	}
}
