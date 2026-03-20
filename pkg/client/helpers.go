package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var timeFormats = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05.000000",
	"2006-01-02 15:04:05.000000000",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05.000-07:00",
	"2006-01-02",
	"01/02/2006 15:04:05",
	"01/02/2006",
	"Jan 02, 2006 15:04:05",
	"02-JAN-2006 15:04:05",
	"02-Jan-2006 15:04:05",
	"02-JAN-06 15:04:05",
	"02-Jan-06 15:04:05",
	"02-01-2006",
	"02-01-06",
	"January 02, 2006 15:04:05",
	"2006-01-02-15.04.05.000000",
	time.UnixDate,
	time.ANSIC,
	time.RubyDate,
	time.RFC822,
	time.RFC822Z,
	time.RFC850,
	time.RFC1123,
	time.RFC1123Z,
	time.Stamp,
	time.StampMilli,
	time.StampMicro,
	time.StampNano,
}

func ParseTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, fmt.Errorf("baton-file: empty time string")
	}

	value = strings.TrimSpace(value)

	for _, format := range timeFormats {
		if t, err := time.Parse(format, value); err == nil {
			return &t, nil
		}
	}

	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		if i > 0 && i < 4102444800 {
			t := time.Unix(i, 0)
			return &t, nil
		}
	}

	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		if i > 1000000000000 && i < 4102444800000 {
			t := time.Unix(i/1000, (i%1000)*1000000)
			return &t, nil
		}
	}

	return nil, fmt.Errorf("baton-file: unable to parse time %q with any known format", value)
}

type FlexibleStringList []string

func (f *FlexibleStringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		if s != "" {
			*f = FlexibleStringList{s}
		}
		return nil
	case yaml.SequenceNode:
		var ss []string
		if err := node.Decode(&ss); err != nil {
			return err
		}
		*f = FlexibleStringList(ss)
		return nil
	case yaml.AliasNode:
		return f.UnmarshalYAML(node.Alias)
	default:
		return nil
	}
}

func (f *FlexibleStringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(data, &s) == nil {
		if s != "" {
			*f = FlexibleStringList{s}
		}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(data, &ss); err != nil {
		return err
	}
	*f = FlexibleStringList(ss)
	return nil
}

func ParseTypeColonID(value string) (string, string, error) {
	idx := strings.Index(value, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("baton-file: invalid format %q, expected \"type:id\"", value)
	}
	return value[:idx], value[idx+1:], nil
}

func SplitCommaSeparated(raw string) FlexibleStringList {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var result FlexibleStringList
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func ParseOptionalBool(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1":
		v := true
		return &v
	case "false", "0":
		v := false
		return &v
	default:
		return nil
	}
}
