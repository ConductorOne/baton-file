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
// The function is required by the connectorbuilder.Connector interface.
func (fc *FileConnector) Metadata(ctx context.Context) (*v2.ConnectorMetadata, error) {
	return &v2.ConnectorMetadata{
		DisplayName: "File Connector",
		Description: "Connector that processes data from a local file",
	}, nil
}

// Validate validates the connector configuration.
// The function is required by the connectorbuilder.Connector interface.
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

// ResourceSyncers returns a list of syncers for the connector.
// The function is required by the connectorbuilder.Connector interface.
// It determines resource types from the input file and creates a syncer instance for each type, enabling the SDK to sync them.
// The implementation loads minimal data to find resource types, builds the type cache, and creates simple syncers passing only the file path for per-sync loading.
func (fc *FileConnector) ResourceSyncers(ctx context.Context) []connectorbuilder.ResourceSyncer {
	l := ctxzap.Extract(ctx)
	l.Info("ResourceSyncers method called", zap.String("input_file_path", fc.inputFilePath))
	loadedData, err := LoadFileData(fc.inputFilePath)
	if err != nil {
		l.Error("Failed to load input data file to determine resource types", zap.Error(err))
		return nil
	}

	resourceTypesCache, err := buildResourceTypeCache(ctx, loadedData.Resources, loadedData.Users)
	if err != nil {
		l.Error("Failed to build resource type cache", zap.Error(err))
		return nil
	}

	rv := make([]connectorbuilder.ResourceSyncer, 0, len(resourceTypesCache))
	for _, rt := range resourceTypesCache {
		rv = append(rv, newFileSyncer(rt, fc.inputFilePath))
	}

	l.Info("Created resource syncers", zap.Int("count", len(rv)))
	return rv
}
