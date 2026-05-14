package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chainreactors/ioa"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	segments := pathSegments(r.URL.Path)
	if len(segments) == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	switch segments[0] {
	case "nodes":
		h.serveNodes(w, r, segments)
	case "spaces":
		h.serveSpaces(w, r, segments)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *Handler) serveNodes(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 1 && r.Method == http.MethodPost {
		h.registerNode(w, r)
		return
	}

	if len(segments) == 1 && r.Method == http.MethodGet {
		h.listNodes(w, r)
		return
	}

	if len(segments) == 2 && r.Method == http.MethodGet {
		h.getNode(w, r, segments[1])
		return
	}

	writeError(w, methodOrNotFound(r.Method, segments, "nodes"), "not found")
}

// registerNode registers a new node.
//
//	@Summary		Register node
//	@Description	Register a new node with the IOA server
//	@Tags			nodes
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ioa.NodeCreate		true	"Node registration payload"
//	@Success		201		{object}	ioa.Node			"Created node"
//	@Failure		422		{object}	ioa.ErrorResponse	"Invalid request body"
//	@Failure		500		{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/nodes [post]
func (h *Handler) registerNode(w http.ResponseWriter, r *http.Request) {
	var body ioa.NodeCreate
	if !decodeJSON(w, r, &body) {
		return
	}
	node, err := h.service.RegisterNode(r.Context(), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, node)
}

// listNodes lists all registered nodes.
//
//	@Summary		List nodes
//	@Description	Return all registered nodes
//	@Tags			nodes
//	@Produce		json
//	@Success		200	{array}		ioa.Node			"List of nodes"
//	@Failure		500	{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/nodes [get]
func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.service.ListNodes(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

// getNode returns a node by ID.
//
//	@Summary		Get node
//	@Description	Return a specific node by its ID
//	@Tags			nodes
//	@Produce		json
//	@Param			nodeID	path		string				true	"Node ID"
//	@Success		200		{object}	ioa.Node			"Node details"
//	@Failure		404		{object}	ioa.ErrorResponse	"Node not found"
//	@Failure		500		{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/nodes/{nodeID} [get]
func (h *Handler) getNode(w http.ResponseWriter, r *http.Request, nodeID string) {
	node, err := h.service.GetNode(r.Context(), nodeID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, node)
}

func (h *Handler) serveSpaces(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 1 && r.Method == http.MethodPost {
		h.createSpace(w, r)
		return
	}

	if len(segments) == 1 && r.Method == http.MethodGet {
		h.listSpaces(w, r)
		return
	}

	if len(segments) < 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	spaceID := segments[1]

	if len(segments) == 2 && r.Method == http.MethodGet {
		h.getSpace(w, r, spaceID)
		return
	}

	if len(segments) == 3 && segments[2] == "messages" {
		switch r.Method {
		case http.MethodPost:
			h.sendMessage(w, r, spaceID)
		case http.MethodGet:
			h.readMessages(w, r, spaceID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if len(segments) == 3 && segments[2] == "sse" && r.Method == http.MethodGet {
		h.sseSpace(w, r, spaceID)
		return
	}

	if len(segments) == 5 && segments[2] == "messages" && segments[4] == "sse" && r.Method == http.MethodGet {
		h.sseMessage(w, r, spaceID, segments[3])
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

// createSpace creates or joins a space.
//
//	@Summary		Create space
//	@Description	Create a new space or join an existing one. If x-node-id header is provided, the caller node joins the space.
//	@Tags			spaces
//	@Accept			json
//	@Produce		json
//	@Param			x-node-id	header		string				false	"Caller node ID"
//	@Param			body		body		ioa.SpaceCreate		true	"Space creation payload"
//	@Success		200			{object}	ioa.SpaceInfo		"Space info"
//	@Failure		422			{object}	ioa.ErrorResponse	"Invalid request body"
//	@Failure		500			{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/spaces [post]
func (h *Handler) createSpace(w http.ResponseWriter, r *http.Request) {
	var body ioa.SpaceCreate
	if !decodeJSON(w, r, &body) {
		return
	}
	info, err := h.service.CreateSpace(r.Context(), callerNodeHeader(r), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// listSpaces lists all spaces.
//
//	@Summary		List spaces
//	@Description	Return all spaces with their node counts and message counts
//	@Tags			spaces
//	@Produce		json
//	@Success		200	{array}		ioa.SpaceInfo		"List of spaces"
//	@Failure		500	{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/spaces [get]
func (h *Handler) listSpaces(w http.ResponseWriter, r *http.Request) {
	spaces, err := h.service.ListSpaces(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, spaces)
}

// getSpace returns a space by ID.
//
//	@Summary		Get space
//	@Description	Return details of a specific space including its nodes and message count
//	@Tags			spaces
//	@Produce		json
//	@Param			spaceID	path		string				true	"Space ID"
//	@Success		200		{object}	ioa.SpaceInfo		"Space details"
//	@Failure		404		{object}	ioa.ErrorResponse	"Space not found"
//	@Failure		500		{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/spaces/{spaceID} [get]
func (h *Handler) getSpace(w http.ResponseWriter, r *http.Request, spaceID string) {
	info, err := h.service.GetSpace(r.Context(), spaceID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// sendMessage sends a message to a space.
//
//	@Summary		Send message
//	@Description	Send a message to the specified space. The sender is identified by the x-node-id header.
//	@Tags			messages
//	@Accept			json
//	@Produce		json
//	@Param			spaceID		path		string				true	"Space ID"
//	@Param			x-node-id	header		string				false	"Caller node ID (sender)"
//	@Param			body		body		ioa.SendMessage		true	"Message payload"
//	@Success		201			{object}	ioa.Message			"Created message"
//	@Failure		404			{object}	ioa.ErrorResponse	"Space not found"
//	@Failure		422			{object}	ioa.ErrorResponse	"Invalid request body"
//	@Failure		500			{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/spaces/{spaceID}/messages [post]
func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request, spaceID string) {
	var body ioa.SendMessage
	if !decodeJSON(w, r, &body) {
		return
	}
	message, err := h.service.SendMessage(r.Context(), spaceID, callerNodeHeader(r), body)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, message)
}

// readMessages reads messages from a space.
//
//	@Summary		Read messages
//	@Description	Read messages from a space. Supports filtering by message context, pagination, and node-scoped reads.
//	@Tags			messages
//	@Produce		json
//	@Param			spaceID		path		string				true	"Space ID"
//	@Param			x-node-id	header		string				false	"Caller node ID (filters messages addressed to this node)"
//	@Param			message_id	query		string				false	"Return the context (ancestors/descendants) of this message"
//	@Param			after		query		string				false	"Pagination cursor: return messages after this ID"
//	@Param			limit		query		int					false	"Maximum number of messages to return"
//	@Param			all			query		bool				false	"If true, return all messages ignoring node filtering"
//	@Success		200			{array}		ioa.Message			"List of messages"
//	@Failure		404			{object}	ioa.ErrorResponse	"Space not found"
//	@Failure		422			{object}	ioa.ErrorResponse	"Invalid query parameters"
//	@Failure		500			{object}	ioa.ErrorResponse	"Internal server error"
//	@Router			/spaces/{spaceID}/messages [get]
func (h *Handler) readMessages(w http.ResponseWriter, r *http.Request, spaceID string) {
	opts, ok := readOptionsFromRequest(w, r)
	if !ok {
		return
	}
	messages, err := h.service.ReadMessages(r.Context(), spaceID, callerNodeHeader(r), opts)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, messages)
}

// sseSpace subscribes to real-time messages in a space via Server-Sent Events.
//
//	@Summary		Subscribe to space events
//	@Description	Open a Server-Sent Events stream for all messages in the specified space
//	@Tags			events
//	@Produce		text/event-stream
//	@Param			spaceID	path	string	true	"Space ID"
//	@Success		200		"SSE stream of ioa.Message events"
//	@Failure		404		{object}	ioa.ErrorResponse	"Space not found"
//	@Failure		500		{object}	ioa.ErrorResponse	"Streaming not supported"
//	@Router			/spaces/{spaceID}/sse [get]
func (h *Handler) sseSpace(w http.ResponseWriter, r *http.Request, spaceID string) {
	h.sse(w, r, spaceID, "")
}

// sseMessage subscribes to real-time events related to a specific message via Server-Sent Events.
//
//	@Summary		Subscribe to message events
//	@Description	Open a Server-Sent Events stream filtered to messages related to the specified message
//	@Tags			events
//	@Produce		text/event-stream
//	@Param			spaceID		path	string	true	"Space ID"
//	@Param			messageID	path	string	true	"Message ID to filter related events"
//	@Success		200			"SSE stream of related ioa.Message events"
//	@Failure		404			{object}	ioa.ErrorResponse	"Space or message not found"
//	@Failure		500			{object}	ioa.ErrorResponse	"Streaming not supported"
//	@Router			/spaces/{spaceID}/messages/{messageID}/sse [get]
func (h *Handler) sseMessage(w http.ResponseWriter, r *http.Request, spaceID, messageID string) {
	h.sse(w, r, spaceID, messageID)
}

func (h *Handler) sse(w http.ResponseWriter, r *http.Request, spaceID, messageID string) {
	if _, err := h.service.GetSpace(r.Context(), spaceID); err != nil {
		writeServiceError(w, err)
		return
	}
	if messageID != "" {
		if _, err := h.service.ReadMessages(r.Context(), spaceID, "", ioa.ReadOptions{MessageID: messageID}); err != nil {
			writeServiceError(w, err)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsubscribe := h.service.Hub().Subscribe(spaceID)
	defer unsubscribe()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if messageID != "" {
				related, err := h.service.IsRelated(r.Context(), spaceID, messageID, msg.ID)
				if err != nil || !related {
					continue
				}
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func readOptionsFromRequest(w http.ResponseWriter, r *http.Request) (ioa.ReadOptions, bool) {
	query := r.URL.Query()
	opts := ioa.ReadOptions{
		MessageID: strings.TrimSpace(query.Get("message_id")),
		After:     strings.TrimSpace(query.Get("after")),
	}
	if query.Get("all") != "" {
		all, err := strconv.ParseBool(query.Get("all"))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "all must be a boolean")
			return ioa.ReadOptions{}, false
		}
		opts.All = all
	}
	if query.Get("limit") != "" {
		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil || limit <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "limit must be greater than 0")
			return ioa.ReadOptions{}, false
		}
		opts.Limit = limit
	}
	return opts, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return false
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusUnprocessableEntity, "request body must contain a single JSON object")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeServiceError(w http.ResponseWriter, err error) {
	writeError(w, statusOf(err), detailOf(err))
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ioa.ErrorResponse{Detail: detail})
}

func pathSegments(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	parts := strings.Split(path, "/")
	result := parts[:0]
	for _, part := range parts {
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func callerNodeHeader(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("x-node-id"))
}

func methodOrNotFound(method string, segments []string, root string) int {
	if len(segments) > 0 && segments[0] == root {
		switch method {
		case http.MethodGet, http.MethodPost:
			return http.StatusNotFound
		default:
			return http.StatusMethodNotAllowed
		}
	}
	return http.StatusNotFound
}
