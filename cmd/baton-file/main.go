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
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var version = "dev"

// --- Baton Connector Mode Flags ---
var inputFileField = field.StringField(
	"input",
	field.WithDescription("Path to the input file"),
	field.WithRequired(true),
	field.WithShortHand("i"),
)

// --- End Baton Connector Mode Flags ---

// ConfigurationFields defines the configuration schema for the connector.
var ConfigurationFields = []field.SchemaField{
	inputFileField,
}

// main is the entry point for the application.
func main() {
	ctx := context.Background()

	// Create a new configuration instance with our defined fields.
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

	// Explicitly allowlist the flags we want to show in --help
	allowedFlags := map[string]bool{
		"input":         true,
		"client-id":     true,
		"client-secret": true,
		"help":          true,
	}

	// Hide flags not in the allowlist
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if !allowedFlags[f.Name] {
			f.Hidden = true
		}
	})
	// Hide persistent flags not in allowlist (includes log-level, file, etc.)
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !allowedFlags[f.Name] {
			f.Hidden = true
		}
	})

	// Add requested shorthands for standard flags
	if pflag := cmd.PersistentFlags().Lookup("client-id"); pflag != nil {
		pflag.Shorthand = "c"
	}
	if pflag := cmd.PersistentFlags().Lookup("client-secret"); pflag != nil {
		pflag.Shorthand = "s"
	}

	// Execute the command.
	err = cmd.Execute()
	if err != nil {
		// Error reporting is handled internally by the SDK/Cobra for common cases,
		// but we catch fatal errors during execution here.
		fmt.Fprintln(os.Stderr, "Error executing command:", err.Error())
		os.Exit(1)
	}
}

// getConnector is the function passed to DefineConfiguration to create the connector server.
// It's called by the SDK's CLI framework when the command is executed.
func getConnector(ctx context.Context, v *viper.Viper) (types.ConnectorServer, error) {
	// Extract the logger configured by the SDK's CLI helpers.
	l := ctxzap.Extract(ctx)

	// Get the input file path from Viper (which reads flags/env vars).
	inputFile := v.GetString(inputFileField.FieldName)
	if inputFile == "" {
		return nil, fmt.Errorf("--input file path is required")
	}

	// Ensure file exists before creating connector.
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("input file not found: %s", inputFile)
	}

	// Create the core File connector instance, passing the path.
	fc, err := connector.NewFileConnector(ctx, inputFile)
	if err != nil {
		// Handle error from NewFileConnector if any validation added
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
