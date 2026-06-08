package settings

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Type string

const (
	TypeString  Type = "string"
	TypeBoolean Type = "boolean"
	TypeInteger Type = "integer"
	TypeNumber  Type = "number"
	TypeJSON    Type = "json"
)

type Validation struct {
	Required  bool     `json:"required,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Options   []string `json:"options,omitempty"`
}

type Schema struct {
	Key        string     `json:"key"`
	Type       Type       `json:"type"`
	Default    any        `json:"default"`
	Sensitive  bool       `json:"sensitive"`
	Writable   bool       `json:"writable"`
	Validation Validation `json:"validation,omitempty"`
	ModuleID   string     `json:"module_id,omitempty"`
}

type Registry struct {
	schemas map[string]Schema
}

var keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

func NewRegistry() *Registry {
	return &Registry{schemas: map[string]Schema{}}
}

func (r *Registry) Register(moduleID string, schemas []Schema) error {
	if r == nil {
		return errors.New("settings registry is nil")
	}
	for _, schema := range schemas {
		schema.ModuleID = strings.TrimSpace(moduleID)
		schema.Key = strings.TrimSpace(schema.Key)
		if schema.Key == "" {
			return errors.New("settings schema key is required")
		}
		if !keyPattern.MatchString(schema.Key) {
			return fmt.Errorf("invalid settings schema key: %s", schema.Key)
		}
		if !validType(schema.Type) {
			return fmt.Errorf("invalid settings schema type for %s: %s", schema.Key, schema.Type)
		}
		if _, exists := r.schemas[schema.Key]; exists {
			return fmt.Errorf("settings schema key already registered: %s", schema.Key)
		}
		r.schemas[schema.Key] = schema
	}
	return nil
}

func (r *Registry) Get(key string) (Schema, bool) {
	if r == nil {
		return Schema{}, false
	}
	schema, ok := r.schemas[strings.TrimSpace(key)]
	return schema, ok
}

func (r *Registry) Schemas() []Schema {
	if r == nil {
		return nil
	}
	items := make([]Schema, 0, len(r.schemas))
	for _, schema := range r.schemas {
		items = append(items, schema)
	}
	slices.SortFunc(items, func(a, b Schema) int {
		return strings.Compare(a.Key, b.Key)
	})
	return items
}

func validType(value Type) bool {
	switch value {
	case TypeString, TypeBoolean, TypeInteger, TypeNumber, TypeJSON:
		return true
	default:
		return false
	}
}
