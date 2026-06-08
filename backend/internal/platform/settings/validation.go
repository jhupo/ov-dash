package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
)

var ErrInvalidValue = errors.New("invalid setting value")

func Decode(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func Validate(schema Schema, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: value is required", ErrInvalidValue)
	}
	value, err := Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid json", ErrInvalidValue)
	}
	if value == nil {
		if schema.Validation.Required {
			return nil, fmt.Errorf("%w: value is required", ErrInvalidValue)
		}
		return nil, nil
	}

	switch schema.Type {
	case TypeString:
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: expected string", ErrInvalidValue)
		}
		if err := validateString(schema.Validation, text); err != nil {
			return nil, err
		}
	case TypeBoolean:
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("%w: expected boolean", ErrInvalidValue)
		}
	case TypeInteger:
		number, ok := value.(json.Number)
		if !ok {
			return nil, fmt.Errorf("%w: expected integer", ErrInvalidValue)
		}
		intValue, err := number.Int64()
		if err != nil {
			return nil, fmt.Errorf("%w: expected integer", ErrInvalidValue)
		}
		if err := validateNumber(schema.Validation, float64(intValue)); err != nil {
			return nil, err
		}
	case TypeNumber:
		number, ok := value.(json.Number)
		if !ok {
			return nil, fmt.Errorf("%w: expected number", ErrInvalidValue)
		}
		floatValue, err := number.Float64()
		if err != nil || math.IsNaN(floatValue) || math.IsInf(floatValue, 0) {
			return nil, fmt.Errorf("%w: expected number", ErrInvalidValue)
		}
		if err := validateNumber(schema.Validation, floatValue); err != nil {
			return nil, err
		}
	case TypeJSON:
		return value, nil
	default:
		return nil, fmt.Errorf("%w: unknown schema type", ErrInvalidValue)
	}
	return value, nil
}

func validateString(validation Validation, value string) error {
	if validation.Required && value == "" {
		return fmt.Errorf("%w: value is required", ErrInvalidValue)
	}
	if validation.MinLength != nil && len(value) < *validation.MinLength {
		return fmt.Errorf("%w: string is too short", ErrInvalidValue)
	}
	if validation.MaxLength != nil && len(value) > *validation.MaxLength {
		return fmt.Errorf("%w: string is too long", ErrInvalidValue)
	}
	if validation.Pattern != "" {
		re, err := regexp.Compile(validation.Pattern)
		if err != nil {
			return fmt.Errorf("%w: invalid validation pattern", ErrInvalidValue)
		}
		if !re.MatchString(value) {
			return fmt.Errorf("%w: string does not match pattern", ErrInvalidValue)
		}
	}
	if len(validation.Options) > 0 && !slices.Contains(validation.Options, value) {
		return fmt.Errorf("%w: value is not an allowed option", ErrInvalidValue)
	}
	return nil
}

func validateNumber(validation Validation, value float64) error {
	if validation.Min != nil && value < *validation.Min {
		return fmt.Errorf("%w: number is too small", ErrInvalidValue)
	}
	if validation.Max != nil && value > *validation.Max {
		return fmt.Errorf("%w: number is too large", ErrInvalidValue)
	}
	return nil
}
