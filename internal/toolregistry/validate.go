package toolregistry

import "fmt"

// ValidateInput checks required fields and basic types against InputSchema.
// Empty schema means "no contract yet" and passes (caller may treat as soft).
func ValidateInput(schema map[string]ParamSchema, input map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if input == nil {
		input = map[string]any{}
	}
	for name, param := range schema {
		val, ok := input[name]
		if !ok || val == nil {
			if param.Required {
				return fmt.Errorf("toolregistry: missing required input %q", name)
			}
			continue
		}
		if param.Type == "" || param.Type == ParamAny {
			continue
		}
		if !valueMatchesType(val, param.Type) {
			return fmt.Errorf("toolregistry: input %q has wrong type (want %s)", name, param.Type)
		}
	}
	return nil
}

func valueMatchesType(v any, t ParamType) bool {
	switch t {
	case ParamString:
		_, ok := v.(string)
		return ok
	case ParamNumber:
		switch v.(type) {
		case float64, float32, int, int32, int64:
			return true
		default:
			return false
		}
	case ParamBoolean:
		_, ok := v.(bool)
		return ok
	case ParamObject:
		_, ok := v.(map[string]any)
		return ok
	case ParamArray:
		switch v.(type) {
		case []any, []string, []map[string]any:
			return true
		default:
			return false
		}
	default:
		return true
	}
}
