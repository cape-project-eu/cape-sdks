package v1

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"cape-project.eu/sdk-generator/provider/pulumi/secapi/models"
	"github.com/gin-gonic/gin"
)

type server struct {
	mu        sync.RWMutex
	instances map[string]models.Instance
}

func RegisterServer(router gin.IRouter) {
	RegisterHandlersWithOptions(router, &server{
		instances: map[string]models.Instance{},
	}, GinServerOptions{
		BaseURL: "/providers/seca.compute",
	})
}

func (s *server) ListSkus(c *gin.Context, _tenant models.TenantPathParam, _params ListSkusParams) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *server) GetSku(c *gin.Context, _tenant models.TenantPathParam, _name models.ResourcePathParam) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *server) ListInstances(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, _params ListInstancesParams) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]models.Instance, 0)
	for _, instance := range s.instances {
		if instance.Metadata == nil {
			continue
		}
		if instance.Metadata.Tenant == tenant && instance.Metadata.Workspace == workspace {
			items = append(items, instance)
		}
	}

	c.JSON(http.StatusOK, InstanceIterator{
		Items: items,
		Metadata: models.ResponseMetadata{
			Provider: "seca.compute/v1",
			Resource: fmt.Sprintf("tenants/%s/workspaces/%s/instances", tenant, workspace),
			Verb:     "list",
		},
	})
}

func (s *server) DeleteInstance(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, _params DeleteInstanceParams) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := instanceKey(tenant, workspace, name)
	if _, ok := s.instances[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	delete(s.instances, key)
	c.JSON(http.StatusAccepted, gin.H{
		"deleted":   true,
		"tenant":    tenant,
		"workspace": workspace,
		"name":      name,
	})
}

func (s *server) GetInstance(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	instance, ok := s.instances[instanceKey(tenant, workspace, name)]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	c.JSON(http.StatusOK, instance)
}

func (s *server) CreateOrUpdateInstance(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, _params CreateOrUpdateInstanceParams) {
	var instance models.Instance
	if err := c.ShouldBindJSON(&instance); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	key := instanceKey(tenant, workspace, name)
	existing, exists := s.instances[key]
	if !exists {
		instance.Metadata = &models.RegionalWorkspaceResourceMetadata{
			ApiVersion:      "v1",
			CreatedAt:       now,
			Kind:            "instance",
			LastModifiedAt:  now,
			Name:            name,
			Provider:        "seca.compute",
			Region:          "global",
			Resource:        fmt.Sprintf("tenants/%s/workspaces/%s/instances/%s", tenant, workspace, name),
			ResourceVersion: 1,
			Tenant:          tenant,
			Verb:            "put",
			Workspace:       workspace,
		}
		setInstanceState(&instance, models.ResourceStatePending)

		s.instances[key] = instance
		version := instance.Metadata.ResourceVersion
		s.scheduleInstanceStateTransition(tenant, workspace, name, version, 100*time.Millisecond, models.ResourceStateCreating)
		s.scheduleInstanceStateTransition(tenant, workspace, name, version, 600*time.Millisecond, models.ResourceStateActive)
		c.JSON(http.StatusCreated, instance)
		return
	}

	setInstanceState(&existing, models.ResourceStateActive)
	s.instances[key] = existing

	if existing.Metadata != nil {
		instance.Metadata = existing.Metadata
	} else {
		instance.Metadata = &models.RegionalWorkspaceResourceMetadata{}
	}

	instance.Metadata.ApiVersion = "v1"
	instance.Metadata.Kind = "instance"
	instance.Metadata.Name = name
	instance.Metadata.Provider = "seca.compute"
	instance.Metadata.Region = "global"
	instance.Metadata.Resource = fmt.Sprintf("tenants/%s/workspaces/%s/instances/%s", tenant, workspace, name)
	instance.Metadata.Tenant = tenant
	instance.Metadata.Verb = "put"
	instance.Metadata.Workspace = workspace

	if instance.Metadata.CreatedAt.IsZero() {
		instance.Metadata.CreatedAt = now
	}
	instance.Metadata.LastModifiedAt = now
	instance.Metadata.ResourceVersion++
	if instance.Metadata.ResourceVersion == 0 {
		instance.Metadata.ResourceVersion = 1
	}
	setInstanceState(&instance, models.ResourceStateUpdating)

	s.instances[key] = instance
	version := instance.Metadata.ResourceVersion
	s.scheduleInstanceStateTransition(tenant, workspace, name, version, 500*time.Millisecond, models.ResourceStateActive)
	c.JSON(http.StatusOK, instance)
}

func (s *server) RestartInstance(c *gin.Context, _tenant models.TenantPathParam, _workspace models.WorkspacePathParam, _name models.ResourcePathParam, _params RestartInstanceParams) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *server) StartInstance(c *gin.Context, _tenant models.TenantPathParam, _workspace models.WorkspacePathParam, _name models.ResourcePathParam, _params StartInstanceParams) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *server) StopInstance(c *gin.Context, _tenant models.TenantPathParam, _workspace models.WorkspacePathParam, _name models.ResourcePathParam, _params StopInstanceParams) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

func (s *server) scheduleInstanceStateTransition(tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, version int64, delay time.Duration, state models.ResourceState) {
	go func() {
		time.Sleep(delay)

		s.mu.Lock()
		defer s.mu.Unlock()

		key := instanceKey(tenant, workspace, name)
		instance, ok := s.instances[key]
		if !ok {
			return
		}

		if instance.Metadata == nil || instance.Metadata.ResourceVersion != version {
			return
		}

		setInstanceState(&instance, state)
		s.instances[key] = instance
	}()
}

func setInstanceState(instance *models.Instance, state models.ResourceState) {
	if instance.Status == nil {
		instance.Status = &models.InstanceStatus{
			Conditions: []models.StatusCondition{},
			PowerState: models.InstanceStatusPowerStateOff,
		}
	}
	if instance.Status.Conditions == nil {
		instance.Status.Conditions = []models.StatusCondition{}
	}
	if instance.Status.State != nil && *instance.Status.State == state {
		return
	}

	instance.Status.State = &state

	msg := fmt.Sprintf("Instance is now in %s state", state)
	reason := "stateChange"
	instance.Status.Conditions = append(instance.Status.Conditions, models.StatusCondition{
		LastTransitionAt: time.Now().UTC(),
		Message:          &msg,
		Reason:           &reason,
		State:            state,
	})
}

func instanceKey(tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam) string {
	return fmt.Sprintf("%s-%s-%s", tenant, workspace, name)
}
