package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vladyslavpavlenko/pheme/internal/api/rest"
	pb "github.com/vladyslavpavlenko/pheme/internal/gen/pheme/v1"
	"github.com/vladyslavpavlenko/pheme/internal/logger"
	"github.com/vladyslavpavlenko/pheme/internal/membership"
)

func TestClusterStatus(t *testing.T) {
	members := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	members.ApplyUpdate(&pb.StateUpdate{
		NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE,
		Version: 1, Zone: "zone-b", Addr: "127.0.0.1:7947",
	})

	handler := rest.NewHandler(members, logger.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/cluster/status", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var nodes []map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&nodes))
	assert.Len(t, nodes, 2)
}

func TestClusterMembers(t *testing.T) {
	members := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	members.ApplyUpdate(&pb.StateUpdate{NodeId: "node-2", State: pb.NodeState_NODE_STATE_ALIVE, Version: 1})
	members.ApplyUpdate(&pb.StateUpdate{NodeId: "node-3", State: pb.NodeState_NODE_STATE_DEAD, Version: 1})

	handler := rest.NewHandler(members, logger.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/cluster/members", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result []map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Len(t, result, 2)
}

func TestHealth(t *testing.T) {
	members := membership.NewList("node-1", "127.0.0.1:7946", "zone-a")
	handler := rest.NewHandler(members, logger.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
