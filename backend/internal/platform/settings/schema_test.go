package settings

import "testing"

func TestRegistryRegisterRejectsDuplicateAndInvalidKeys(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("test", []Schema{{Key: "test.value", Type: TypeString}}); err != nil {
		t.Fatalf("register valid schema: %v", err)
	}
	if err := registry.Register("test", []Schema{{Key: "test.value", Type: TypeString}}); err == nil {
		t.Fatal("expected duplicate key error")
	}
	if err := registry.Register("test", []Schema{{Key: "Bad/Value", Type: TypeString}}); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestValidateTypesAndStringRules(t *testing.T) {
	maxLength := 3
	schema := Schema{
		Key:  "test.value",
		Type: TypeString,
		Validation: Validation{
			Required:  true,
			MaxLength: &maxLength,
		},
	}
	if _, err := Validate(schema, []byte(`"ok"`)); err != nil {
		t.Fatalf("validate string: %v", err)
	}
	if _, err := Validate(schema, []byte(`""`)); err == nil {
		t.Fatal("expected required string error")
	}
	if _, err := Validate(schema, []byte(`"long"`)); err == nil {
		t.Fatal("expected max length error")
	}
	if _, err := Validate(Schema{Key: "test.flag", Type: TypeBoolean}, []byte(`true`)); err != nil {
		t.Fatalf("validate boolean: %v", err)
	}
	if _, err := Validate(Schema{Key: "test.flag", Type: TypeBoolean}, []byte(`"true"`)); err == nil {
		t.Fatal("expected boolean type error")
	}
}
