package v1

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"cape-project.eu/mockserver/models"
	"github.com/gin-gonic/gin"
)

type server struct {
	mu            sync.RWMutex
	blockStorages map[string]models.BlockStorage
	images        map[string]models.Image
}

type storageSKUDefinition struct {
	name          string
	tier          string
	iops          int
	storageType   models.StorageSkuSpecType
	minVolumeSize int
}

var storageSKUCatalog = []storageSKUDefinition{
	{name: "seca.rd100", tier: "RD100", iops: 100, storageType: models.StorageSkuTypeRemoteDurable, minVolumeSize: 50},
	{name: "seca.rd500", tier: "RD500", iops: 500, storageType: models.StorageSkuTypeRemoteDurable, minVolumeSize: 50},
	{name: "seca.rd2k", tier: "RD2K", iops: 2000, storageType: models.StorageSkuTypeRemoteDurable, minVolumeSize: 50},
	{name: "seca.rd10k", tier: "RD10K", iops: 10000, storageType: models.StorageSkuTypeRemoteDurable, minVolumeSize: 50},
	{name: "seca.rd20k", tier: "RD20K", iops: 20000, storageType: models.StorageSkuTypeRemoteDurable, minVolumeSize: 50},
	{name: "seca.ld100", tier: "LD100", iops: 100, storageType: models.StorageSkuTypeLocalDurable, minVolumeSize: 50},
	{name: "seca.ld500", tier: "LD500", iops: 500, storageType: models.StorageSkuTypeLocalDurable, minVolumeSize: 50},
	{name: "seca.ld5k", tier: "LD5K", iops: 5000, storageType: models.StorageSkuTypeLocalDurable, minVolumeSize: 50},
	{name: "seca.ld20k", tier: "LD20K", iops: 20000, storageType: models.StorageSkuTypeLocalDurable, minVolumeSize: 50},
	{name: "seca.ld40k", tier: "LD40K", iops: 40000, storageType: models.StorageSkuTypeLocalDurable, minVolumeSize: 50},
	{name: "seca.le100", tier: "LE100", iops: 100, storageType: models.StorageSkuTypeLocalEphemeral, minVolumeSize: 50},
	{name: "seca.le500", tier: "LE500", iops: 500, storageType: models.StorageSkuTypeLocalEphemeral, minVolumeSize: 50},
	{name: "seca.le5k", tier: "LE5K", iops: 5000, storageType: models.StorageSkuTypeLocalEphemeral, minVolumeSize: 50},
	{name: "seca.le20k", tier: "LE20K", iops: 20000, storageType: models.StorageSkuTypeLocalEphemeral, minVolumeSize: 50},
	{name: "seca.le40k", tier: "LE40K", iops: 40000, storageType: models.StorageSkuTypeLocalEphemeral, minVolumeSize: 50},
}

func RegisterServer(router gin.IRouter) {
	RegisterHandlersWithOptions(router, &server{
		blockStorages: map[string]models.BlockStorage{},
		images:        map[string]models.Image{},
	}, GinServerOptions{
		BaseURL: "/providers/seca.storage",
	})
}

func (s *server) ListImages(c *gin.Context, tenant models.TenantPathParam, _params ListImagesParams) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]models.Image, 0)
	for _, image := range s.images {
		if image.Metadata == nil {
			continue
		}
		if image.Metadata.Tenant == tenant {
			items = append(items, image)
		}
	}

	c.JSON(http.StatusOK, ImageIterator{
		Items: items,
		Metadata: models.ResponseMetadata{
			Provider: "seca.storage/v1",
			Resource: fmt.Sprintf("tenants/%s/images", tenant),
			Verb:     "list",
		},
	})
}

func (s *server) DeleteImage(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam, _params DeleteImageParams) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := imageKey(tenant, name)
	if _, ok := s.images[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	delete(s.images, key)
	c.JSON(http.StatusAccepted, gin.H{
		"deleted": true,
		"tenant":  tenant,
		"name":    name,
	})
}

func (s *server) GetImage(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	image, ok := s.images[imageKey(tenant, name)]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "image not found"})
		return
	}

	c.JSON(http.StatusOK, image)
}

func (s *server) CreateOrUpdateImage(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam, _params CreateOrUpdateImageParams) {
	var image models.Image
	if err := c.ShouldBindJSON(&image); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	key := imageKey(tenant, name)
	existing, exists := s.images[key]
	if !exists {
		image.Metadata = &models.RegionalResourceMetadata{
			ApiVersion:      "v1",
			CreatedAt:       now,
			Kind:            "image",
			LastModifiedAt:  now,
			Name:            name,
			Provider:        "seca.storage",
			Region:          "global",
			Resource:        fmt.Sprintf("tenants/%s/images/%s", tenant, name),
			ResourceVersion: 1,
			Tenant:          tenant,
			Verb:            "put",
		}
		setImageState(&image, models.ResourceStatePending)

		s.images[key] = image
		version := image.Metadata.ResourceVersion
		s.scheduleImageStateTransition(tenant, name, version, 100*time.Millisecond, models.ResourceStateCreating)
		s.scheduleImageStateTransition(tenant, name, version, 600*time.Millisecond, models.ResourceStateActive)
		c.JSON(http.StatusCreated, image)
		return
	}

	setImageState(&existing, models.ResourceStateActive)
	s.images[key] = existing

	if existing.Metadata != nil {
		image.Metadata = existing.Metadata
	} else {
		image.Metadata = &models.RegionalResourceMetadata{}
	}

	image.Metadata.ApiVersion = "v1"
	image.Metadata.Kind = "image"
	image.Metadata.Name = name
	image.Metadata.Provider = "seca.storage"
	image.Metadata.Region = "global"
	image.Metadata.Resource = fmt.Sprintf("tenants/%s/images/%s", tenant, name)
	image.Metadata.Tenant = tenant
	image.Metadata.Verb = "put"

	if image.Metadata.CreatedAt.IsZero() {
		image.Metadata.CreatedAt = now
	}
	image.Metadata.LastModifiedAt = now
	image.Metadata.ResourceVersion++
	if image.Metadata.ResourceVersion == 0 {
		image.Metadata.ResourceVersion = 1
	}
	setImageState(&image, models.ResourceStateUpdating)

	s.images[key] = image
	version := image.Metadata.ResourceVersion
	s.scheduleImageStateTransition(tenant, name, version, 500*time.Millisecond, models.ResourceStateActive)
	c.JSON(http.StatusOK, image)
}

func (s *server) scheduleImageStateTransition(tenant models.TenantPathParam, name models.ResourcePathParam, version int64, delay time.Duration, state models.ResourceState) {
	go func() {
		time.Sleep(delay)

		s.mu.Lock()
		defer s.mu.Unlock()

		key := imageKey(tenant, name)
		image, ok := s.images[key]
		if !ok {
			return
		}

		if image.Metadata == nil || image.Metadata.ResourceVersion != version {
			return
		}

		setImageState(&image, state)
		s.images[key] = image
	}()
}

func setImageState(image *models.Image, state models.ResourceState) {
	if image.Status == nil {
		image.Status = &models.ImageStatus{
			Conditions: []models.StatusCondition{},
		}
	}
	if image.Status.Conditions == nil {
		image.Status.Conditions = []models.StatusCondition{}
	}
	if image.Status.State == state {
		return
	}

	image.Status.State = state

	image.Status.Conditions = append(image.Status.Conditions, models.StatusCondition{
		LastTransitionAt: time.Now().UTC(),
		Message:          fmt.Sprintf("Image is now in %s state", state),
		Reason:           "stateChange",
		State:            state,
	})
}

func imageKey(tenant models.TenantPathParam, name models.ResourcePathParam) string {
	return fmt.Sprintf("%s-%s", tenant, name)
}

func (s *server) ListSkus(c *gin.Context, tenant models.TenantPathParam, params ListSkusParams) {
	skus := make([]models.StorageSku, 0, len(storageSKUCatalog))
	for _, def := range storageSKUCatalog {
		sku := storageSKUFromDefinition(tenant, def)
		if params.Labels != nil && !matchesLabelSelector(sku.Labels, string(*params.Labels)) {
			continue
		}
		skus = append(skus, sku)
	}

	c.JSON(http.StatusOK, SkuIterator{
		Items: skus,
		Metadata: models.ResponseMetadata{
			Provider: "seca.storage/v1",
			Resource: fmt.Sprintf("tenants/%s/skus", tenant),
			Verb:     "list",
		},
	})
}

func (s *server) GetSku(c *gin.Context, tenant models.TenantPathParam, name models.ResourcePathParam) {
	for _, def := range storageSKUCatalog {
		if def.name == name {
			c.JSON(http.StatusOK, storageSKUFromDefinition(tenant, def))
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "sku not found"})
}

func (s *server) ListBlockStorages(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, _params ListBlockStoragesParams) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]models.BlockStorage, 0)
	for _, blockStorage := range s.blockStorages {
		if blockStorage.Metadata == nil {
			continue
		}
		if blockStorage.Metadata.Tenant == tenant && blockStorage.Metadata.Workspace == workspace {
			items = append(items, blockStorage)
		}
	}

	c.JSON(http.StatusOK, BlockStorageIterator{
		Items: items,
		Metadata: models.ResponseMetadata{
			Provider: "seca.storage/v1",
			Resource: fmt.Sprintf("tenants/%s/workspaces/%s/block-storages", tenant, workspace),
			Verb:     "list",
		},
	})
}

func (s *server) DeleteBlockStorage(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, _params DeleteBlockStorageParams) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := blockStorageKey(tenant, workspace, name)
	if _, ok := s.blockStorages[key]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "block-storage not found"})
		return
	}

	delete(s.blockStorages, key)
	c.JSON(http.StatusAccepted, gin.H{
		"deleted":   true,
		"tenant":    tenant,
		"workspace": workspace,
		"name":      name,
	})
}

func (s *server) GetBlockStorage(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	blockStorage, ok := s.blockStorages[blockStorageKey(tenant, workspace, name)]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "block-storage not found"})
		return
	}

	c.JSON(http.StatusOK, blockStorage)
}

func (s *server) CreateOrUpdateBlockStorage(c *gin.Context, tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, _params CreateOrUpdateBlockStorageParams) {
	var blockStorage models.BlockStorage
	if err := c.ShouldBindJSON(&blockStorage); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	key := blockStorageKey(tenant, workspace, name)
	existing, exists := s.blockStorages[key]
	if !exists {
		blockStorage.Metadata = &models.RegionalWorkspaceResourceMetadata{
			ApiVersion:      "v1",
			CreatedAt:       now,
			Kind:            "block-storage",
			LastModifiedAt:  now,
			Name:            name,
			Provider:        "seca.storage",
			Region:          "global",
			Resource:        fmt.Sprintf("tenants/%s/workspaces/%s/block-storages/%s", tenant, workspace, name),
			ResourceVersion: 1,
			Tenant:          tenant,
			Verb:            "put",
			Workspace:       workspace,
		}
		setBlockStorageState(&blockStorage, models.ResourceStatePending)

		s.blockStorages[key] = blockStorage
		version := blockStorage.Metadata.ResourceVersion
		s.scheduleBlockStorageStateTransition(tenant, workspace, name, version, 100*time.Millisecond, models.ResourceStateCreating)
		s.scheduleBlockStorageStateTransition(tenant, workspace, name, version, 600*time.Millisecond, models.ResourceStateActive)
		c.JSON(http.StatusCreated, blockStorage)
		return
	}

	setBlockStorageState(&existing, models.ResourceStateActive)
	s.blockStorages[key] = existing

	if existing.Metadata != nil {
		blockStorage.Metadata = existing.Metadata
	} else {
		blockStorage.Metadata = &models.RegionalWorkspaceResourceMetadata{}
	}

	blockStorage.Metadata.ApiVersion = "v1"
	blockStorage.Metadata.Kind = "block-storage"
	blockStorage.Metadata.Name = name
	blockStorage.Metadata.Provider = "seca.storage"
	blockStorage.Metadata.Region = "global"
	blockStorage.Metadata.Resource = fmt.Sprintf("tenants/%s/workspaces/%s/block-storages/%s", tenant, workspace, name)
	blockStorage.Metadata.Tenant = tenant
	blockStorage.Metadata.Verb = "put"
	blockStorage.Metadata.Workspace = workspace

	if blockStorage.Metadata.CreatedAt.IsZero() {
		blockStorage.Metadata.CreatedAt = now
	}
	blockStorage.Metadata.LastModifiedAt = now
	blockStorage.Metadata.ResourceVersion++
	if blockStorage.Metadata.ResourceVersion == 0 {
		blockStorage.Metadata.ResourceVersion = 1
	}
	setBlockStorageState(&blockStorage, models.ResourceStateUpdating)

	s.blockStorages[key] = blockStorage
	version := blockStorage.Metadata.ResourceVersion
	s.scheduleBlockStorageStateTransition(tenant, workspace, name, version, 500*time.Millisecond, models.ResourceStateActive)
	c.JSON(http.StatusOK, blockStorage)
}

func (s *server) scheduleBlockStorageStateTransition(tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam, version int64, delay time.Duration, state models.ResourceState) {
	go func() {
		time.Sleep(delay)

		s.mu.Lock()
		defer s.mu.Unlock()

		key := blockStorageKey(tenant, workspace, name)
		blockStorage, ok := s.blockStorages[key]
		if !ok {
			return
		}

		if blockStorage.Metadata == nil || blockStorage.Metadata.ResourceVersion != version {
			return
		}

		setBlockStorageState(&blockStorage, state)
		s.blockStorages[key] = blockStorage
	}()
}

func setBlockStorageState(blockStorage *models.BlockStorage, state models.ResourceState) {
	if blockStorage.Status == nil {
		blockStorage.Status = &models.BlockStorageStatus{
			Conditions: []models.StatusCondition{},
			SizeGB:     blockStorage.Spec.SizeGB,
		}
	}
	if blockStorage.Status.Conditions == nil {
		blockStorage.Status.Conditions = []models.StatusCondition{}
	}
	if blockStorage.Status.State == state {
		return
	}

	blockStorage.Status.SizeGB = blockStorage.Spec.SizeGB
	blockStorage.Status.State = state

	blockStorage.Status.Conditions = append(blockStorage.Status.Conditions, models.StatusCondition{
		LastTransitionAt: time.Now().UTC(),
		Message:          fmt.Sprintf("BlockStorage is now in %s state", state),
		Reason:           "stateChange",
		State:            state,
	})
}

func blockStorageKey(tenant models.TenantPathParam, workspace models.WorkspacePathParam, name models.ResourcePathParam) string {
	return fmt.Sprintf("%s-%s-%s", tenant, workspace, name)
}

func storageSKUFromDefinition(tenant models.TenantPathParam, def storageSKUDefinition) models.StorageSku {
	return models.StorageSku{
		Labels: models.Labels{
			"provider":      "seca",
			"tier":          def.tier,
			"type":          string(def.storageType),
			"iops":          strconv.Itoa(def.iops),
			"minVolumeSize": strconv.Itoa(def.minVolumeSize),
		},
		Metadata: &models.SkuResourceMetadata{
			ApiVersion: "v1",
			Kind:       models.SkuResourceMetadataKindResourceKindStorageSku,
			Name:       def.name,
			Provider:   "seca.storage/v1",
			Region:     "eu-central-1",
			Resource:   fmt.Sprintf("tenants/%s/skus/%s", tenant, def.name),
			Tenant:     tenant,
			Verb:       "get",
		},
		Spec: &models.StorageSkuSpec{
			Iops:          def.iops,
			MinVolumeSize: def.minVolumeSize,
			Type:          def.storageType,
		},
	}
}

func matchesLabelSelector(labels models.Labels, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return true
	}

	filters := strings.SplitSeq(selector, ",")
	for filter := range filters {
		if !matchesLabelFilter(labels, strings.TrimSpace(filter)) {
			return false
		}
	}
	return true
}

func matchesLabelFilter(labels models.Labels, filter string) bool {
	if filter == "" {
		return true
	}

	operators := []string{"!=", ">=", "<=", "=", ">", "<"}
	for _, operator := range operators {
		if !strings.Contains(filter, operator) {
			continue
		}

		parts := strings.SplitN(filter, operator, 2)
		if len(parts) != 2 {
			return false
		}

		keyPattern := strings.TrimSpace(parts[0])
		valuePattern := strings.TrimSpace(parts[1])
		if keyPattern == "" {
			return false
		}

		switch operator {
		case "=":
			for key, value := range labels {
				if matchPattern(keyPattern, key) && matchPattern(valuePattern, value) {
					return true
				}
			}
			return false
		case "!=":
			for key, value := range labels {
				if matchPattern(keyPattern, key) && matchPattern(valuePattern, value) {
					return false
				}
			}
			return true
		default:
			target, err := strconv.ParseFloat(valuePattern, 64)
			if err != nil {
				return false
			}
			for key, value := range labels {
				if !matchPattern(keyPattern, key) {
					continue
				}
				current, err := strconv.ParseFloat(value, 64)
				if err != nil {
					continue
				}
				switch operator {
				case ">":
					if current > target {
						return true
					}
				case "<":
					if current < target {
						return true
					}
				case ">=":
					if current >= target {
						return true
					}
				case "<=":
					if current <= target {
						return true
					}
				}
			}
			return false
		}
	}

	return false
}

func matchPattern(pattern string, value string) bool {
	if strings.Contains(pattern, "*") {
		regex := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		re, err := regexp.Compile(regex)
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	return value == pattern
}
