package main

import (
	"context"
	"fmt"
	"os"

	"github.com/conductorone/baton-file/pkg/connector"

	"github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/field"
	"github.com/conductorone/baton-sdk/pkg/types"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var version = "dev"

var inputFileField = field.StringField(
	"input",
	field.WithDescription("Path to the input file"),
	field.WithRequired(true),
	field.WithShortHand("i"),
)

var ConfigurationFields = []field.SchemaField{
	inputFileField,
}

func main() {
	ctx := context.Background()

	cfg := field.NewConfiguration(ConfigurationFields)

	// Define the CLI configuration using the Baton SDK helper.
	// The sets up the command, flags (including defaults like --client-id, --file),
	// environment variable binding, and the main execution logic.
	_, cmd, err := config.DefineConfiguration(
		ctx,
		"baton-file",
		getConnector,
		cfg,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error defining configuration:", err.Error())
		os.Exit(1)
	}

	// Set command usage details.
	cmd.Use = "baton-file"
	cmd.Short = "Process data files (xlsx, yaml, json) into Baton resources"
	cmd.Long = `baton-file processes structured data files (.xlsx, .yaml, .json) containing resource, entitlement, and grant data.

It expects the data to be organized into specific sheets (Excel) or top-level keys (YAML/JSON): 'users', 'resources', 'entitlements', 'grants'.

By default (without --client-id and --client-secret flags), it generates a C1Z file compatible with ConductorOne.
If authentication flags are provided, it runs as a direct connector.`
	cmd.Version = version

	if pflag := cmd.PersistentFlags().Lookup("client-id"); pflag != nil {
		pflag.Shorthand = "c"
	}
	if pflag := cmd.PersistentFlags().Lookup("client-secret"); pflag != nil {
		pflag.Shorthand = "s"
	}

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error executing command:", err.Error())
		os.Exit(1)
	}
}

// getConnector is the function passed to DefineConfiguration to create the connector server.
// It's called by the SDK's CLI framework when the command is executed.
func getConnector(ctx context.Context, v *viper.Viper) (types.ConnectorServer, error) {
	// Extract the logger configured by the SDK's CLI helpers.
	l := ctxzap.Extract(ctx)

	inputFile := v.GetString(inputFileField.FieldName)
	if inputFile == "" {
		return nil, fmt.Errorf("--input file path is required")
	}

	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file not found: %s", inputFile)
	}

	fc, err := connector.NewFileConnector(ctx, inputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create file connector: %w", err)
	}

	// Use the connector builder to create the gRPC server instance.
	// The wraps our FileConnector.
	c, err := connectorbuilder.NewConnector(ctx, fc)
	if err != nil {
		l.Error("Error creating connector server", zap.Error(err))
		return nil, fmt.Errorf("failed to create connector server: %w", err)
	}

	return c, nil
}
