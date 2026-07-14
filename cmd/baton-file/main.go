package main

import (
	"context"

	"github.com/conductorone/baton-file/pkg/connector"
	sdkConfig "github.com/conductorone/baton-sdk/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/connectorrunner"
	"github.com/conductorone/baton-sdk/pkg/field"
)

var version = "dev"

var (
	inputFileField = field.StringField(
		"input",
		field.WithDescription("Path to the input data file (.yaml, .yml, .json, .jsonc, .xlsx, .csv)"),
		field.WithRequired(true),
		field.WithShortHand("i"),
	)

	// SupportsExternalResources enables the "shared identity source" feature:
	// C1 surfaces the identity-source picker for this connector, and the SDK
	// registers the --external-resource-c1z / --external-resource-entitlement-id-filter
	// flags used to resolve external_grants match rules against another
	// connector's synced principals.
	Configuration = field.NewConfiguration(
		[]field.SchemaField{
			inputFileField,
		},
		field.WithSupportsExternalResources(true),
	)
)

func main() {
	ctx := context.Background()
	sdkConfig.RunConnector(ctx, "baton-file", version, Configuration, connector.New,
		connectorrunner.WithDefaultCapabilitiesConnectorBuilderV2(&connector.StaticCapabilitiesConnector{}),
	)
}
