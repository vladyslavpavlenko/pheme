package rest

import (
	"encoding/json"
	"net/http"

	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

type Handler struct {
	members *membership.List
	l       *logger.Logger
}

func NewHandler(members *membership.List, l *logger.Logger) *Handler {
	return &Handler{members: members, l: l}
}

type nodeResponse struct {
	ID      string `json:"id"`
	Addr    string `json:"addr"`
	Zone    string `json:"zone"`
	State   string `json:"state"`
	Version uint64 `json:"version"`
}

func stateString(s pb.NodeState) string {
	switch s {
	case pb.NodeState_NODE_STATE_ALIVE:
		return "alive"
	case pb.NodeState_NODE_STATE_SUSPECT:
		return "suspect"
	case pb.NodeState_NODE_STATE_DEAD:
		return "dead"
	case pb.NodeState_NODE_STATE_LEFT:
		return "left"
	default:
		return "unknown"
	}
}

func nodeToResponse(n *membership.Node) nodeResponse {
	return nodeResponse{
		ID:      n.ID,
		Addr:    n.Addr,
		Zone:    n.Zone,
		State:   stateString(n.State),
		Version: n.Version,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /cluster/status", h.ClusterStatus)
	mux.HandleFunc("GET /cluster/members", h.ClusterMembers)
	mux.HandleFunc("GET /health", h.Health)
}

func (h *Handler) ClusterStatus(w http.ResponseWriter, _ *http.Request) {
	nodes := h.members.AllNodes()
	resp := make([]nodeResponse, 0, len(nodes))
	for _, n := range nodes {
		resp = append(resp, nodeToResponse(n))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ClusterMembers(w http.ResponseWriter, _ *http.Request) {
	nodes := h.members.AliveMembers()
	resp := make([]nodeResponse, 0, len(nodes))
	for _, n := range nodes {
		resp = append(resp, nodeToResponse(n))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
