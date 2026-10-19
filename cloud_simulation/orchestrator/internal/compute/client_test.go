// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package compute

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

const (
	instancesPath  = "/compute/v1/projects/test-project/zones/test-zone/instances"
	operationsPath = "/compute/v1/projects/test-project/zones/test-zone/operations"
)

// fakeComputeAPI serves the Compute Engine REST calls of the Client from an
// in-memory set of instances, keyed by instance name.
type fakeComputeAPI struct {
	mu        sync.Mutex
	instances map[string]map[string]any
	// failGet makes every instance GET fail with HTTP 500.
	failGet bool
	// vanishDuringDelete makes the deletion operation of every instance
	// report HTTP 404, as when another process deleted the instance first.
	vanishDuringDelete bool
	// listFilter is the filter of the last list request.
	listFilter string
}

func (f *fakeComputeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if strings.HasPrefix(r.URL.Path, operationsPath) {
		// Deletions finish at once, so every operation is done.
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, operationsPath+"/"), "/wait")
		if f.vanishDuringDelete {
			writeNotFound(w, name)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"name": name, "status": "DONE", "zone": "test-zone"})
		return
	}

	if !strings.HasPrefix(r.URL.Path, instancesPath) {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, instancesPath), "/")
	switch {
	case r.Method == http.MethodGet && f.failGet:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]any{"code": 500, "message": "backend error"}})
	case r.Method == http.MethodGet && name == "":
		f.listFilter = r.URL.Query().Get("filter")
		wantFilter := `labels.` + simulationLabelKey + ` = "` + simulationLabelValue + `"`
		items := []map[string]any{}
		for _, instance := range f.instances {
			labels, _ := instance["labels"].(map[string]string)
			if f.listFilter == "" || (f.listFilter == wantFilter && labels[simulationLabelKey] == simulationLabelValue) {
				items = append(items, instance)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case r.Method == http.MethodGet:
		instance, ok := f.instances[name]
		if !ok {
			writeNotFound(w, name)
			return
		}
		writeJSON(w, http.StatusOK, instance)
	case r.Method == http.MethodDelete:
		if _, ok := f.instances[name]; !ok {
			writeNotFound(w, name)
			return
		}
		delete(f.instances, name)
		writeJSON(w, http.StatusOK, map[string]any{"name": "operation-delete-" + name, "status": "DONE", "zone": "test-zone"})
	default:
		http.Error(w, "unexpected request "+r.Method+" "+r.URL.Path, http.StatusNotImplemented)
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeNotFound(w http.ResponseWriter, name string) {
	writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"code": 404, "message": "instance " + name + " not found"}})
}

func instance(name string, created time.Time) map[string]any {
	return map[string]any{
		"name":              name,
		"creationTimestamp": created.Format(time.RFC3339),
		"labels":            map[string]string{simulationLabelKey: simulationLabelValue},
	}
}

// newTestClient returns a Client that talks to api over a local HTTP server.
func newTestClient(t *testing.T, api *fakeComputeAPI) *Client {
	t.Helper()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	instancesClient, err := compute.NewInstancesRESTClient(context.Background(), option.WithEndpoint(srv.URL), option.WithoutAuthentication())
	require.NoError(t, err)
	c := &Client{client: instancesClient, logger: slog.Default()}
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func TestQueryInstancesReturnsSimulationVMsOlderThanTheLimit(t *testing.T) {
	now := time.Now()
	api := &fakeComputeAPI{instances: map[string]map[string]any{
		"sim-old":     instance("sim-old", now.Add(-5*time.Hour)),
		"sim-new":     instance("sim-new", now.Add(-1*time.Hour)),
		"other-old":   {"name": "other-old", "creationTimestamp": now.Add(-5 * time.Hour).Format(time.RFC3339)},
		"sim-invalid": {"name": "sim-invalid", "creationTimestamp": "not a timestamp", "labels": map[string]string{simulationLabelKey: simulationLabelValue}},
	}}
	c := newTestClient(t, api)

	got, err := c.QueryInstances(context.Background(), "test-project", "test-zone", 4)
	require.NoError(t, err)
	assert.Equal(t, []string{"sim-old"}, got)
	assert.Equal(t, `labels.kind = "simulation"`, api.listFilter)

	api.failGet = true
	_, err = c.QueryInstances(context.Background(), "test-project", "test-zone", 4)
	assert.ErrorContains(t, err, "error iterating over instances")
}

func TestGetInstance(t *testing.T) {
	created := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	api := &fakeComputeAPI{instances: map[string]map[string]any{"sim-1": instance("sim-1", created)}}
	c := newTestClient(t, api)

	got, err := c.GetInstance(context.Background(), "test-project", "test-zone", "sim-1")
	require.NoError(t, err)
	assert.Equal(t, "sim-1", got.GetName())
	assert.Equal(t, created.Format(time.RFC3339), got.GetCreationTimestamp())

	_, err = c.GetInstance(context.Background(), "test-project", "test-zone", "unknown")
	assert.ErrorContains(t, err, "unable to get instance details for unknown")

	_, err = c.GetInstance(context.Background(), "wrong-project", "test-zone", "sim-1")
	assert.ErrorContains(t, err, "unable to get instance details for sim-1")
}

func TestInstanceExists(t *testing.T) {
	api := &fakeComputeAPI{instances: map[string]map[string]any{"sim-1": instance("sim-1", time.Now())}}
	c := newTestClient(t, api)
	ctx := context.Background()

	exists, err := c.InstanceExists(ctx, "test-project", "test-zone", "sim-1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = c.InstanceExists(ctx, "test-project", "test-zone", "unknown")
	require.NoError(t, err)
	assert.False(t, exists)

	api.failGet = true
	_, err = c.InstanceExists(ctx, "test-project", "test-zone", "sim-1")
	assert.ErrorContains(t, err, "failed to check existence for instance sim-1")
}

func TestDeleteInstance(t *testing.T) {
	api := &fakeComputeAPI{instances: map[string]map[string]any{"sim-1": instance("sim-1", time.Now())}}
	c := newTestClient(t, api)

	ctx := context.Background()

	require.NoError(t, c.DeleteInstance(ctx, "test-project", "test-zone", "sim-1"))
	assert.NotContains(t, api.instances, "sim-1")

	// An instance that is already gone counts as deleted.
	assert.NoError(t, c.DeleteInstance(ctx, "test-project", "test-zone", "sim-1"))
}

func TestDeleteInstanceThatVanishesDuringDeletion(t *testing.T) {
	api := &fakeComputeAPI{
		instances:          map[string]map[string]any{"sim-1": instance("sim-1", time.Now())},
		vanishDuringDelete: true,
	}
	c := newTestClient(t, api)

	assert.NoError(t, c.DeleteInstance(context.Background(), "test-project", "test-zone", "sim-1"))
}
