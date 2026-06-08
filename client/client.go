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

	"github.com/chainreactors/ioa"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	nodeID     string
	token      string
	accessKey  string
}

func NewClient(baseURL string, nodeID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid ioa url: %s", baseURL)
	}
	return &Client{
		baseURL:    parsed,
		httpClient: http.DefaultClient,
		nodeID:     nodeID,
	}, nil
}

func NewClientWithToken(baseURL string, token string) (*Client, error) {
	c, err := NewClient(baseURL, "")
	if err != nil {
		return nil, err
	}
	c.token = token
	return c, nil
}

func (c *Client) NodeID() string {
	return c.nodeID
}

func (c *Client) Register(ctx context.Context, accessKey, name, description string, meta map[string]interface{}) (ioa.AuthResponse, error) {
	body := ioa.AuthRegister{Name: name, Description: description, AccessKey: accessKey, Meta: meta}
	var resp ioa.AuthResponse
	if err := c.do(ctx, http.MethodPost, "/auth/register", nil, body, &resp); err != nil {
		return ioa.AuthResponse{}, err
	}
	c.token = resp.Token
	c.nodeID = resp.ID
	c.accessKey = accessKey
	return resp, nil
}

func (c *Client) ListSpaces(ctx context.Context) ([]ioa.SpaceInfo, error) {
	var spaces []ioa.SpaceInfo
	if err := c.do(ctx, http.MethodGet, "/spaces", nil, nil, &spaces); err != nil {
		return nil, err
	}
	return spaces, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]ioa.Node, error) {
	var nodes []ioa.Node
	if err := c.do(ctx, http.MethodGet, "/nodes", nil, nil, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *Client) ListMessages(ctx context.Context, filter ioa.MessageFilter) ([]ioa.MessageRecord, error) {
	endpoint := endpointWithQuery("/messages", messageFilterValues(filter))
	var messages []ioa.MessageRecord
	if err := c.do(ctx, http.MethodGet, endpoint, nil, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) GetGraph(ctx context.Context, opts ioa.GraphOptions) (ioa.GraphView, error) {
	endpoint := endpointWithQuery("/graph", graphOptionsValues(opts))
	var graph ioa.GraphView
	if err := c.do(ctx, http.MethodGet, endpoint, nil, nil, &graph); err != nil {
		return ioa.GraphView{}, err
	}
	return graph, nil
}

func (c *Client) GetSpaceGraph(ctx context.Context, spaceID string, opts ioa.GraphOptions) (ioa.GraphView, error) {
	opts.SpaceID = ""
	endpoint := endpointWithQuery("/spaces/"+url.PathEscape(spaceID)+"/graph", graphOptionsValues(opts))
	var graph ioa.GraphView
	if err := c.do(ctx, http.MethodGet, endpoint, nil, nil, &graph); err != nil {
		return ioa.GraphView{}, err
	}
	return graph, nil
}

func (c *Client) GetSpaceInfo(ctx context.Context, spaceID string) (ioa.SpaceInfo, error) {
	var info ioa.SpaceInfo
	if err := c.do(ctx, http.MethodGet, "/spaces/"+url.PathEscape(spaceID), nil, nil, &info); err != nil {
		return ioa.SpaceInfo{}, err
	}
	return info, nil
}

func (c *Client) ResolveSpace(ctx context.Context, nameOrID string) (ioa.SpaceInfo, error) {
	info, err := c.GetSpaceInfo(ctx, nameOrID)
	if err == nil {
		return info, nil
	}
	if pe, ok := err.(*ioa.Error); !ok || pe.Status != http.StatusNotFound {
		return ioa.SpaceInfo{}, err
	}
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return ioa.SpaceInfo{}, err
	}
	for _, s := range spaces {
		if s.Name == nameOrID {
			return s, nil
		}
	}
	return ioa.SpaceInfo{}, ioa.ProtocolError(http.StatusNotFound, "space %q not found", nameOrID)
}

func (c *Client) ReadPublic(ctx context.Context, spaceID string, opts ioa.ReadOptions) ([]ioa.Message, error) {
	var messages []ioa.Message
	if err := c.do(ctx, http.MethodGet, readEndpoint(spaceID, opts), nil, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) RegisterNode(ctx context.Context, name, description string, meta map[string]interface{}) (ioa.Node, error) {
	var node ioa.Node
	if err := c.do(ctx, http.MethodPost, "/nodes", nil, ioa.NodeCreate{Name: name, Description: description, Meta: meta}, &node); err != nil {
		return ioa.Node{}, err
	}
	c.nodeID = node.ID
	return node, nil
}

func (c *Client) Space(ctx context.Context, name, description string, tags ...string) (ioa.SpaceInfo, error) {
	if c.nodeID == "" {
		return ioa.SpaceInfo{}, fmt.Errorf("No node: call register_node() first")
	}
	headers := map[string]string{"X-Node-ID": c.nodeID}
	if c.accessKey != "" {
		headers["X-Access-Key"] = c.accessKey
	}
	var info ioa.SpaceInfo
	if err := c.do(ctx, http.MethodPost, "/spaces", headers, ioa.SpaceCreate{Name: name, Description: description, Tags: tags}, &info); err != nil {
		return ioa.SpaceInfo{}, err
	}
	return info, nil
}

func (c *Client) Send(ctx context.Context, spaceID string, body ioa.SendMessage) (ioa.Message, error) {
	if c.nodeID == "" {
		return ioa.Message{}, fmt.Errorf("No sender: call register_node() first")
	}
	var message ioa.Message
	if err := c.do(ctx, http.MethodPost, "/spaces/"+url.PathEscape(spaceID)+"/messages", map[string]string{"X-Node-ID": c.nodeID}, body, &message); err != nil {
		return ioa.Message{}, err
	}
	return message, nil
}

func (c *Client) Read(ctx context.Context, spaceID string, opts ioa.ReadOptions) ([]ioa.Message, error) {
	if c.nodeID == "" {
		return nil, fmt.Errorf("No node: call register_node() first")
	}
	var messages []ioa.Message
	if err := c.do(ctx, http.MethodGet, readEndpoint(spaceID, opts), map[string]string{"X-Node-ID": c.nodeID}, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (c *Client) Subscribe(ctx context.Context, spaceID string) (<-chan ioa.Message, <-chan error, func(), error) {
	target := *c.baseURL
	target.Path = path.Join(c.baseURL.Path, "/spaces/"+url.PathEscape(spaceID)+"/sse")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
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
			return nil, nil, nil, ioa.ProtocolError(resp.StatusCode, "%s", payload.Detail)
		}
		return nil, nil, nil, ioa.ProtocolError(resp.StatusCode, "%s", strings.TrimSpace(string(data)))
	}

	messages := make(chan ioa.Message, 16)
	errs := make(chan error, 1)
	done := make(chan struct{})
	cancel := func() {
		close(done)
		_ = resp.Body.Close()
	}

	go func() {
		defer close(messages)
		defer close(errs)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var data strings.Builder
		for scanner.Scan() {
			select {
			case <-done:
				return
			default:
			}
			line := scanner.Text()
			if line == "" {
				if data.Len() > 0 {
					var msg ioa.Message
					if err := json.Unmarshal([]byte(data.String()), &msg); err != nil {
						errs <- err
						return
					}
					select {
					case messages <- msg:
					case <-done:
						return
					case <-ctx.Done():
						return
					}
					data.Reset()
				}
				continue
			}
			if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
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
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
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
			return ioa.ProtocolError(resp.StatusCode, "%s", payload.Detail)
		}
		return ioa.ProtocolError(resp.StatusCode, "%s", strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func messageFilterValues(filter ioa.MessageFilter) url.Values {
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

func graphOptionsValues(opts ioa.GraphOptions) url.Values {
	values := messageFilterValues(opts.MessageFilter)
	if len(opts.Include) > 0 {
		values.Set("include", strings.Join(opts.Include, ","))
	}
	return values
}

func readEndpoint(spaceID string, opts ioa.ReadOptions) string {
	values := url.Values{}
	if opts.MessageID != "" {
		values.Set("message_id", opts.MessageID)
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
