package connector

import (
	"context"
	"fmt"
	"os"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

// Metadata returns the connector's metadata.
// function is required by the connectorbuilder.Connector interface.
func (fc *FileConnector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "File Connector",
		Description: "Connector that processes data from a local file",
	}, nil
}

// Validate validates the connector configuration.
// function is required by the connectorbuilder.Connector interface.
func (fc *FileConnector) Validate(ctx context.Context) (annotations.Annotations, error) {
	_, err := os.Stat(fc.inputFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("input file not found: %s", fc.inputFilePath)
		}
		return nil, fmt.Errorf("error accessing input file: %w", err)
	}
	return nil, nil
}

// getCachedData returns cached file data and built caches, loading them if not already cached.
// This method is thread-safe and ensures the file is only loaded and processed once.
func (fc *FileConnector) getCachedData(ctx context.Context) (*LoadedData, map[string]*v2.ResourceType, map[string]*v2.Resource, map[string]*v2.Entitlement, map[string]map[string]struct{}, error) {
	l := ctxzap.Extract(ctx)

	// Fast path: check if cache is already populated using read lock
	fc.cacheMutex.RLock()
	if fc.cachedData != nil {
		loadedData := fc.cachedData
		resourceTypes := fc.cachedResourceTypes
		resources := fc.cachedResources
		entitlements := fc.cachedEntitlements
		childTypes := fc.cachedChildTypes
		fc.cacheMutex.RUnlock()
		return loadedData, resourceTypes, resources, entitlements, childTypes, nil
	}
	fc.cacheMutex.RUnlock()

	// Slow path: need to load and build caches, acquire write lock
	fc.cacheMutex.Lock()
	defer fc.cacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine may have loaded it)
	if fc.cachedData != nil {
		return fc.cachedData, fc.cachedResourceTypes, fc.cachedResources, fc.cachedEntitlements, fc.cachedChildTypes, nil
	}

	l.Info("Loading and caching file data", zap.String("input_file_path", fc.inputFilePath))

	// Load file data
	loadedData, err := LoadFileData(fc.inputFilePath)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to load data file: %w", err)
	}

	// Build all caches
	resourceTypesCache, err := buildResourceTypeCache(ctx, loadedData.Resources, loadedData.Users)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to build resource type cache: %w", err)
	}

	resourceCache, err := buildResourceCache(ctx, loadedData.Users, loadedData.Resources, resourceTypesCache)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to build resource cache: %w", err)
	}

	entitlementCache, err := buildEntitlementCache(ctx, loadedData.Entitlements, resourceCache)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("failed to build entitlement cache: %w", err)
	}

	// Build child types index to optimize child type lookups
	childTypesIndex := buildChildTypesIndex(ctx, loadedData.Resources, resourceCache)

	// Store in cache
	fc.cachedData = loadedData
	fc.cachedResourceTypes = resourceTypesCache
	fc.cachedResources = resourceCache
	fc.cachedEntitlements = entitlementCache
	fc.cachedChildTypes = childTypesIndex

	l.Info("Successfully cached file data",
		zap.Int("users", len(loadedData.Users)),
		zap.Int("resources", len(loadedData.Resources)),
		zap.Int("entitlements", len(loadedData.Entitlements)),
		zap.Int("grants", len(loadedData.Grants)))

	return loadedData, resourceTypesCache, resourceCache, entitlementCache, childTypesIndex, nil
}

// ResourceSyncers returns a list of syncers for the connector.
// function is required by the connectorbuilder.Connector interface.
// It determines resource types from the input file and creates a syncer instance for each type, enabling the SDK to sync them.
// implementation loads minimal data to find resource types, builds the type cache, and creates simple syncers passing the connector reference.
func (fc *FileConnector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	l := ctxzap.Extract(ctx)
	l.Info("ResourceSyncers method called", zap.String("input_file_path", fc.inputFilePath))

	_, resourceTypesCache, _, _, _, err := fc.getCachedData(ctx)
	if err != nil {
		l.Error("Failed to load and cache data", zap.Error(err))
		return nil
	}

	rv := make([]connectorbuilder.ResourceSyncer, 0, len(resourceTypesCache))
	for _, rt := range resourceTypesCache {
		rv = append(rv, newFileSyncer(rt, fc))
	}

	l.Info("Created resource syncers", zap.Int("count", len(rv)))
	return rv
}
