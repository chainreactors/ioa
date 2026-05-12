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
		return
	}

	if len(segments) == 1 && r.Method == http.MethodGet {
		nodes, err := h.service.ListNodes(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, nodes)
		return
	}

	if len(segments) == 2 && r.Method == http.MethodGet {
		node, err := h.service.GetNode(r.Context(), segments[1])
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, node)
		return
	}

	writeError(w, methodOrNotFound(r.Method, segments, "nodes"), "not found")
}

func (h *Handler) serveSpaces(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 1 && r.Method == http.MethodPost {
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
		return
	}

	if len(segments) == 1 && r.Method == http.MethodGet {
		spaces, err := h.service.ListSpaces(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, spaces)
		return
	}

	if len(segments) < 2 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	spaceID := segments[1]

	if len(segments) == 2 && r.Method == http.MethodGet {
		info, err := h.service.GetSpace(r.Context(), spaceID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
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
		h.sse(w, r, spaceID, "")
		return
	}

	if len(segments) == 5 && segments[2] == "messages" && segments[4] == "sse" && r.Method == http.MethodGet {
		h.sse(w, r, spaceID, segments[3])
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

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
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
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
