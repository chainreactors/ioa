package protocols

import (
	"fmt"
	"net/url"
	"strings"
)

// NodeRef is a portable node identity. Authority is the public,
// credential-free base URL of the system that owns the node namespace.
type NodeRef struct {
	ID        string `json:"id"`
	Authority string `json:"authority"`
}

func (r NodeRef) Valid() bool {
	return strings.TrimSpace(r.ID) != "" && strings.TrimSpace(r.Authority) != ""
}

// URI returns the canonical, resolvable identifier for the node.
func (r NodeRef) URI() string {
	if !r.Valid() {
		return ""
	}
	return strings.TrimRight(r.Authority, "/") + "/nodes/" + url.PathEscape(r.ID)
}

// CanonicalAuthority removes credentials and request-specific URL components.
func CanonicalAuthority(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid node authority %q", raw)
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/"), nil
}

// IdentityBinding associates an identity from another system with one IOA
// node. Namespace and Subject form the globally unique lookup key; Claims are
// opaque, non-secret descriptive data owned by the integrating system.
type IdentityBinding struct {
	Namespace string         `json:"namespace"`
	Subject   string         `json:"subject"`
	Claims    map[string]any `json:"claims,omitempty"`
}

// Identity is the single extension point for embedding a system identity into
// IOA. Implementations stay owned by the integrating system; IOA only consumes
// the binding during node registration.
type Identity interface {
	IOABinding() IdentityBinding
}

func NormalizeIdentityBindings(bindings []IdentityBinding) ([]IdentityBinding, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(bindings))
	out := make([]IdentityBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.Namespace = strings.TrimSpace(binding.Namespace)
		binding.Subject = strings.TrimSpace(binding.Subject)
		if binding.Namespace == "" || binding.Subject == "" {
			return nil, fmt.Errorf("identity namespace and subject are required")
		}
		key := binding.Namespace + "\x00" + binding.Subject
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate identity %q/%q", binding.Namespace, binding.Subject)
		}
		seen[key] = struct{}{}
		out = append(out, binding)
	}
	return out, nil
}
