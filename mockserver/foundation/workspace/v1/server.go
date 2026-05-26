package v1

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"cape-project.eu/mockserver/models"
	"github.com/gin-gonic/gin"
)

type server struct {
	mu         sync.RWMutex
	workspaces map[string]models.Workspace
}

func RegisterServer(router gin.IRouter) {
	RegisterHandlersWithOptions(router, &server{
		workspaces: map[string]models.Workspace{},
	}, GinServerOptions{
		BaseURL: "/providers/seca.workspace",
	})
}

func (s *server) ListWorkspaces(c *gin.Context, tenant models.TenantPathParam, _params ListWorkspacesParams) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]models.Workspace, 0)
	for _, workspace := range s.workspaces {
		if workspace.Metadata == nil {
			continue
		}
		if workspace.Metadata.Tenant == tenant {
			items = append(items, workspace)
		}
	}

	c.JSON(http.StatusOK, WorkspaceIterator{
		Items: items,
		Metadata: models.ResponseMetadata{
			Provider: "seca.workspace/v1",
			Resource: fmt.Sprintf("tenants/%s/workspaces", tenant),
			Verb:     "list",
		},
	})
}

func (s *server) DeleteWorkspace(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam, _params DeleteWorkspaceParams) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := workspaceKey(tenant, name)
	if _, ok := s.workspaces[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	delete(s.workspaces, key)
	c.JSON(http.StatusAccepted, gin.H{
		"deleted": true,
		"tenant":  tenant,
		"name":    name,
	})
}

func (s *server) GetWorkspace(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workspace, ok := s.workspaces[workspaceKey(tenant, name)]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}

	c.JSON(http.StatusOK, workspace)
}

func (s *server) CreateOrUpdateWorkspace(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam, _params CreateOrUpdateWorkspaceParams) {
	var workspace models.Workspace
	if err := c.ShouldBindJSON(&workspace); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	key := workspaceKey(tenant, name)
	existing, exists := s.workspaces[key]
	if !exists {
		workspace.Metadata = &models.RegionalResourceMetadata{
			ApiVersion:      "v1",
			CreatedAt:       now,
			Kind:            "workspace",
			LastModifiedAt:  now,
			Name:            name,
			Provider:        "seca.workspace",
			Region:          "global",
			Resource:        fmt.Sprintf("tenants/%s/workspaces/%s", tenant, name),
			ResourceVersion: 1,
			Tenant:          tenant,
			Verb:            "put",
		}
		setWorkspaceState(&workspace, models.ResourceStatePending)

		s.workspaces[key] = workspace
		version := workspace.Metadata.ResourceVersion
		s.scheduleWorkspaceStateTransition(tenant, name, version, 100*time.Millisecond, models.ResourceStateCreating)
		s.scheduleWorkspaceStateTransition(tenant, name, version, 600*time.Millisecond, models.ResourceStateActive)
		c.JSON(http.StatusCreated, workspace)
		return
	}

	setWorkspaceState(&existing, models.ResourceStateActive)
	s.workspaces[key] = existing

	if existing.Metadata != nil {
		workspace.Metadata = existing.Metadata
	} else {
		workspace.Metadata = &models.RegionalResourceMetadata{}
	}

	workspace.Metadata.ApiVersion = "v1"
	workspace.Metadata.Kind = "workspace"
	workspace.Metadata.Name = name
	workspace.Metadata.Provider = "seca.workspace"
	workspace.Metadata.Region = "global"
	workspace.Metadata.Resource = fmt.Sprintf("tenants/%s/workspaces/%s", tenant, name)
	workspace.Metadata.Tenant = tenant
	workspace.Metadata.Verb = "put"

	if workspace.Metadata.CreatedAt.IsZero() {
		workspace.Metadata.CreatedAt = now
	}
	workspace.Metadata.LastModifiedAt = now
	workspace.Metadata.ResourceVersion++
	if workspace.Metadata.ResourceVersion == 0 {
		workspace.Metadata.ResourceVersion = 1
	}
	setWorkspaceState(&workspace, models.ResourceStateUpdating)

	s.workspaces[key] = workspace
	version := workspace.Metadata.ResourceVersion
	s.scheduleWorkspaceStateTransition(tenant, name, version, 500*time.Millisecond, models.ResourceStateActive)
	c.JSON(http.StatusOK, workspace)
}

func (s *server) scheduleWorkspaceStateTransition(tenant models.TenantPathParam, name models.ResourcePathParam, version int64, delay time.Duration, state models.ResourceState) {
	go func() {
		time.Sleep(delay)

		s.mu.Lock()
		defer s.mu.Unlock()

		key := workspaceKey(tenant, name)
		workspace, ok := s.workspaces[key]
		if !ok {
			return
		}

		if workspace.Metadata == nil || workspace.Metadata.ResourceVersion != version {
			return
		}

		setWorkspaceState(&workspace, state)
		s.workspaces[key] = workspace
	}()
}

func setWorkspaceState(workspace *models.Workspace, state models.ResourceState) {
	if workspace.Status == nil {
		workspace.Status = &models.WorkspaceStatus{
			Conditions: []models.StatusCondition{},
		}
	}
	if workspace.Status.Conditions == nil {
		workspace.Status.Conditions = []models.StatusCondition{}
	}
	if workspace.Status.State == state {
		return
	}

	workspace.Status.State = state

	workspace.Status.Conditions = append(workspace.Status.Conditions, models.StatusCondition{
		LastTransitionAt: time.Now().UTC(),
		Message:          fmt.Sprintf("Workspace is now in %s state", state),
		Reason:           "stateChange",
		State:            state,
	})
}

func workspaceKey(tenant models.TenantPathParam, name models.ResourcePathParam) string {
	return fmt.Sprintf("%s-%s", tenant, name)
}
