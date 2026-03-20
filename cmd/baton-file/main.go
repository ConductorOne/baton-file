package main

import (
	"context"

	"github.com/conductorone/baton-file/pkg/connector"
	sdkConfig "github.com/conductorone/baton-sdk/pkg/config"
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

	Configuration = field.NewConfiguration([]field.SchemaField{
		inputFileField,
	})
)

func main() {
	ctx := context.Background()
	sdkConfig.RunConnector(ctx, "baton-file", version, Configuration, connector.New)
}
