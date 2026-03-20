package connector

import (
	"strings"

	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var userResourceType = &v2.ResourceType{
	Id:          "user",
	DisplayName: "User",
	Traits:      []v2.ResourceType_Trait{v2.ResourceType_TRAIT_USER},
}

var TraitMap = map[string]v2.ResourceType_Trait{
	"user":   v2.ResourceType_TRAIT_USER,
	"group":  v2.ResourceType_TRAIT_GROUP,
	"role":   v2.ResourceType_TRAIT_ROLE,
	"app":    v2.ResourceType_TRAIT_APP,
	"secret": v2.ResourceType_TRAIT_SECRET,
}

func buildDynamicResourceType(typeID string, traitStr string) *v2.ResourceType {
	traits := []v2.ResourceType_Trait{}
	if t, ok := TraitMap[strings.ToLower(traitStr)]; ok {
		traits = append(traits, t)
	}
	return &v2.ResourceType{
		Id:          strings.ToLower(typeID),
		DisplayName: cases.Title(language.English).String(strings.ToLower(typeID)),
		Traits:      traits,
	}
}
