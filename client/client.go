package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/chainreactors/ioa/protocols"
)

type Client struct {
	mu            sync.RWMutex
	baseURL       *url.URL
	httpClient    *http.Client
	nodeID        string
	token         string
	accessKey     string
	authority     string
	bound         bool
	identities    []protocols.IdentityBinding
	identityDirty bool
}

func NewClient(baseURL string, nodeID string) (*Client, error) {
	return newClient(baseURL, nodeID, true)
}

func newClient(baseURL string, nodeID string, generateID bool) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid ioa url: %s", baseURL)
	}
	var accessKey string
	if parsed.User != nil {
		accessKey = parsed.User.Username()
		parsed.User = nil
	}
	authority, err := protocols.CanonicalAuthority(parsed.String())
	if err != nil {
		return nil, err
	}
	c := &Client{
		baseURL:    parsed,
		httpClient: http.DefaultClient,
		nodeID:     nodeID,
		accessKey:  accessKey,
		authority:  authority,
		bound:      nodeID != "",
	}
	if nodeID == "" && generateID {
		c.nodeID = protocols.NewID()
	}
	return c, nil
}

func NewClientWithToken(baseURL string, token string) (*Client, error) {
	c, err := newClient(baseURL, "", false)
	if err != nil {
		return nil, err
	}
	c.token = token
	return c, nil
}

func (c *Client) NodeID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodeID
}

func (c *Client) NodeRef() protocols.NodeRef {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return protocols.NodeRef{ID: c.nodeID, Authority: c.authority}
}

func (c *Client) Bound() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bound
}

func (c *Client) AccessKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessKey
}

// Bind attaches an integrating system's identity to this client in memory.
// The binding is sent on the next registration and can resolve an existing IOA
// node even when this process started with a fresh local node ID.
func (c *Client) Bind(identity protocols.Identity) error {
	if identity == nil {
		return fmt.Errorf("identity is required")
	}
	bindings, err := protocols.NormalizeIdentityBindings([]protocols.IdentityBinding{identity.IOABinding()})
	if err != nil {
		return err
	}
	binding := bindings[0]
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, existing := range c.identities {
		if existing.Namespace == binding.Namespace && existing.Subject == binding.Subject {
			c.identities[i] = binding
			c.identityDirty = true
			return nil
		}
	}
	c.identities = append(c.identities, binding)
	c.identityDirty = true
	return nil
}

func (c *Client) identityBindings() []protocols.IdentityBinding {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]protocols.IdentityBinding(nil), c.identities...)
}

func (c *Client) EnsureRegistered(ctx context.Context, name, description string, meta map[string]interface{}) error {
	c.mu.RLock()
	bound, dirty := c.bound, c.identityDirty
	c.mu.RUnlock()
	if bound && !dirty {
		return nil
	}
	if c.AccessKey() != "" {
		_, err := c.Register(ctx, c.AccessKey(), name, description, meta)
		return err
	}
	_, err := c.RegisterNode(ctx, name, description, meta)
	return err
}

func (c *Client) Register(ctx context.Context, accessKey, name, description string, meta map[string]interface{}) (protocols.AuthResponse, error) {
	body := protocols.AuthRegister{
		ID: c.NodeID(), Name: name, Description: description,
		AccessKey: accessKey, Meta: meta, Identities: c.identityBindings(),
	}
	var resp protocols.AuthResponse
	if err := c.do(ctx, http.MethodPost, "/auth/register", nil, body, &resp); err != nil {
		return protocols.AuthResponse{}, err
	}
	c.mu.Lock()
	c.token = resp.Token
	c.nodeID = resp.ID
	c.accessKey = accessKey
	c.bound = true
	c.identityDirty = false
	c.mu.Unlock()
	return resp, nil
}

func (c *Client) ListSpaces(ctx context.Context) ([]protocols.SpaceInfo, error) {
	var spaces []protocols.SpaceInfo
	if err := c.do(ctx, http.MethodGet, "/spaces", nil, nil, &spaces); err != nil {
		return nil, err
	}
	return spaces, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]protocols.Node, error) {
	var nodes []protocols.Node
	if err := c.do(ctx, http.MethodGet, "/nodes", nil, nil, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Client) ListMessages(ctx context.Context, filter protocols.MessageFilter) ([]protocols.Message, error) {
	endpoint := endpointWithQuery("/messages", messageFilterValues(filter))
	var messages []protocols.Message
	if err := c.do(ctx, http.MethodGet, endpoint, nil, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) GetSpaceInfo(ctx context.Context, spaceID string) (protocols.SpaceInfo, error) {
	var info protocols.SpaceInfo
	if err := c.do(ctx, http.MethodGet, "/spaces/"+url.PathEscape(spaceID), nil, nil, &info); err != nil {
		return protocols.SpaceInfo{}, err
	}
	return info, nil
}

func (c *Client) ResolveSpace(ctx context.Context, nameOrID string) (protocols.SpaceInfo, error) {
	info, err := c.GetSpaceInfo(ctx, nameOrID)
	if err == nil {
		return info, nil
	}
	if pe, ok := err.(*protocols.Error); !ok || pe.Status != http.StatusNotFound {
		return protocols.SpaceInfo{}, err
	}
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return protocols.SpaceInfo{}, err
	}
	for _, s := range spaces {
		if s.Name == nameOrID {
			return s, nil
		}
	}
	return protocols.SpaceInfo{}, protocols.ProtocolError(http.StatusNotFound, "space %q not found", nameOrID)
}

func (c *Client) ReadPublic(ctx context.Context, spaceID string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	var messages []protocols.Message
	if err := c.do(ctx, http.MethodGet, readEndpoint(spaceID, opts), nil, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) RegisterNode(ctx context.Context, name, description string, meta map[string]interface{}) (protocols.Node, error) {
	var node protocols.Node
	body := protocols.NodeCreate{
		ID: c.NodeID(), Name: name, Description: description,
		Meta: meta, Identities: c.identityBindings(),
	}
	if err := c.do(ctx, http.MethodPost, "/nodes", nil, body, &node); err != nil {
		return protocols.Node{}, err
	}
	c.mu.Lock()
	c.nodeID = node.ID
	c.bound = true
	c.identityDirty = false
	c.mu.Unlock()
	return node, nil
}

func (c *Client) ResolveIdentity(ctx context.Context, namespace, subject string) (protocols.Node, error) {
	values := url.Values{"namespace": {namespace}, "subject": {subject}}
	var node protocols.Node
	err := c.do(ctx, http.MethodGet, endpointWithQuery("/nodes/resolve", values), nil, nil, &node)
	return node, err
}

func (c *Client) UpsertIdentity(ctx context.Context, binding protocols.IdentityBinding) (protocols.Node, error) {
	var node protocols.Node
	nodeID := c.NodeID()
	err := c.do(ctx, http.MethodPut, "/nodes/"+url.PathEscape(nodeID)+"/identities", map[string]string{"X-Node-ID": nodeID}, binding, &node)
	return node, err
}

func (c *Client) DeleteIdentity(ctx context.Context, namespace, subject string) (protocols.Node, error) {
	values := url.Values{"namespace": {namespace}, "subject": {subject}}
	var node protocols.Node
	nodeID := c.NodeID()
	err := c.do(ctx, http.MethodDelete, endpointWithQuery("/nodes/"+url.PathEscape(nodeID)+"/identities", values), map[string]string{"X-Node-ID": nodeID}, nil, &node)
	return node, err
}

func (c *Client) Space(ctx context.Context, name, description string, tags ...string) (protocols.SpaceInfo, error) {
	nodeID := c.NodeID()
	if nodeID == "" {
		return protocols.SpaceInfo{}, fmt.Errorf("No node: call register_node() first")
	}
	headers := map[string]string{"X-Node-ID": nodeID}
	if accessKey := c.AccessKey(); accessKey != "" {
		headers["X-Access-Key"] = accessKey
	}
	var info protocols.SpaceInfo
	if err := c.do(ctx, http.MethodPost, "/spaces", headers, protocols.SpaceCreate{Name: name, Description: description, Tags: tags}, &info); err != nil {
		return protocols.SpaceInfo{}, err
	}
	return info, nil
}

func (c *Client) Send(ctx context.Context, spaceID string, body protocols.SendMessage) (protocols.Message, error) {
	nodeID := c.NodeID()
	if nodeID == "" {
		return protocols.Message{}, fmt.Errorf("No sender: call register_node() first")
	}
	var message protocols.Message
	if err := c.do(ctx, http.MethodPost, "/spaces/"+url.PathEscape(spaceID)+"/messages", map[string]string{"X-Node-ID": nodeID}, body, &message); err != nil {
		return protocols.Message{}, err
	}
	return message, nil
}

func (c *Client) Read(ctx context.Context, spaceID string, opts protocols.ReadOptions) ([]protocols.Message, error) {
	nodeID := c.NodeID()
	if nodeID == "" {
		return nil, fmt.Errorf("No node: call register_node() first")
	}
	var messages []protocols.Message
	if err := c.do(ctx, http.MethodGet, readEndpoint(spaceID, opts), map[string]string{"X-Node-ID": nodeID}, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) Subscribe(ctx context.Context, spaceID string, opts ...SubscribeOption) (<-chan protocols.Message, <-chan error, func(), error) {
	target := *c.baseURL

	var cfg subscribeConfig
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.MessageID != "" {
		target.Path = path.Join(c.baseURL.Path, "/spaces/"+url.PathEscape(spaceID)+"/messages/"+url.PathEscape(cfg.MessageID)+"/sse")
	} else {
		target.Path = path.Join(c.baseURL.Path, "/spaces/"+url.PathEscape(spaceID)+"/sse")
	}
	if cfg.Head != "" || cfg.ForkDepth > 0 {
		q := target.Query()
		if cfg.Head != "" {
			q.Set("head", cfg.Head)
		}
		if cfg.ForkDepth > 0 {
			q.Set("fork_depth", strconv.Itoa(cfg.ForkDepth))
		}
		target.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		var payload struct {
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(data, &payload); err == nil && payload.Detail != "" {
			return nil, nil, nil, protocols.ProtocolError(resp.StatusCode, "%s", payload.Detail)
		}
		return nil, nil, nil, protocols.ProtocolError(resp.StatusCode, "%s", strings.TrimSpace(string(data)))
	}

	messages := make(chan protocols.Message, 16)
	errs := make(chan error, 1)
	done := make(chan struct{})
	var once sync.Once
	cancel := func() {
		once.Do(func() { close(done); _ = resp.Body.Close() })
	}

	go func() {
		defer close(messages)
		defer close(errs)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var data strings.Builder
		var eventType string
		for scanner.Scan() {
			select {
			case <-done:
				return
			default:
			}
			line := scanner.Text()
			if line == "" {
				if data.Len() > 0 {
					var msg protocols.Message
					if err := json.Unmarshal([]byte(data.String()), &msg); err != nil {
						errs <- err
						return
					}
					if eventType == "fork" {
						msg.ContentType = "ioa/fork"
					}
					select {
					case messages <- msg:
					case <-done:
						return
					case <-ctx.Done():
						return
					}
					data.Reset()
					eventType = ""
				}
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(value)
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			select {
			case errs <- err:
			default:
			}
		}
	}()

	return messages, errs, cancel, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, headers map[string]string, body interface{}, out interface{}) error {
	target := *c.baseURL
	target.Path = path.Join(c.baseURL.Path, endpoint)
	if strings.HasSuffix(endpoint, "/") && !strings.HasSuffix(target.Path, "/") {
		target.Path += "/"
	}
	if i := strings.Index(endpoint, "?"); i >= 0 {
		target.Path = path.Join(c.baseURL.Path, endpoint[:i])
		target.RawQuery = endpoint[i+1:]
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, target.String(), reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var payload struct {
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(data, &payload); err == nil && payload.Detail != "" {
			return protocols.ProtocolError(resp.StatusCode, "%s", payload.Detail)
		}
		return protocols.ProtocolError(resp.StatusCode, "%s", strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func messageFilterValues(filter protocols.MessageFilter) url.Values {
	values := url.Values{}
	if filter.SpaceID != "" {
		values.Set("space_id", filter.SpaceID)
	}
	if filter.MessageID != "" {
		values.Set("message_id", filter.MessageID)
	}
	if filter.NodeID != "" {
		values.Set("node_id", filter.NodeID)
	}
	if filter.Sender != "" {
		values.Set("sender", filter.Sender)
	}
	if filter.RefMessage != "" {
		values.Set("ref_message", filter.RefMessage)
	}
	if filter.RefNode != "" {
		values.Set("ref_node", filter.RefNode)
	}
	if filter.After != "" {
		values.Set("after", filter.After)
	}
	if filter.Limit > 0 {
		values.Set("limit", strconv.Itoa(filter.Limit))
	}
	return values
}

func readEndpoint(spaceID string, opts protocols.ReadOptions) string {
	values := url.Values{}
	if opts.MessageID != "" {
		values.Set("message_id", opts.MessageID)
	}
	if opts.Direction != "" {
		values.Set("direction", opts.Direction)
	}
	if opts.After != "" {
		values.Set("after", opts.After)
	}
	if opts.Limit > 0 {
		values.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.All {
		values.Set("all", "true")
	}
	return endpointWithQuery("/spaces/"+url.PathEscape(spaceID)+"/messages", values)
}

func endpointWithQuery(endpoint string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return endpoint + "?" + encoded
	}
	return endpoint
}
