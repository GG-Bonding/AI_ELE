package extractor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agent-experience-engine/agent-experience-engine/internal/experience"
)

// candidateEnvelope is the LLM JSON payload shape.
type candidateEnvelope struct {
	Experiences []experience.Candidate `json:"experiences"`
}

// ValidateCandidatesJSON validates raw LLM JSON against the experience candidates schema.
func ValidateCandidatesJSON(raw []byte) ([]experience.Candidate, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty json payload")
	}

	var probe any
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if err := validateAgainstSchema(probe); err != nil {
		return nil, err
	}

	var env candidateEnvelope
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("decode candidates: %w", err)
	}
	if env.Experiences == nil {
		env.Experiences = []experience.Candidate{}
	}

	for i, c := range env.Experiences {
		if !c.Type.Valid() {
			return nil, fmt.Errorf("experiences[%d].type is invalid: %q", i, c.Type)
		}
		if !c.Scope.Valid() {
			return nil, fmt.Errorf("experiences[%d].scope is invalid: %q", i, c.Scope)
		}
		if strings.TrimSpace(c.Trigger) == "" {
			return nil, fmt.Errorf("experiences[%d].trigger is required", i)
		}
		if strings.TrimSpace(c.Content) == "" {
			return nil, fmt.Errorf("experiences[%d].content is required", i)
		}
		if c.Confidence < 0 || c.Confidence > 1 {
			return nil, fmt.Errorf("experiences[%d].confidence out of range: %v", i, c.Confidence)
		}
	}
	return env.Experiences, nil
}

func validateAgainstSchema(doc any) error {
	obj, ok := doc.(map[string]any)
	if !ok {
		return fmt.Errorf("schema: root must be an object")
	}
	for k := range obj {
		if k != "experiences" {
			return fmt.Errorf("schema: unexpected property %q", k)
		}
	}
	rawList, ok := obj["experiences"]
	if !ok {
		return fmt.Errorf("schema: missing required property experiences")
	}
	list, ok := rawList.([]any)
	if !ok {
		return fmt.Errorf("schema: experiences must be an array")
	}
	for i, item := range list {
		itemObj, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("schema: experiences[%d] must be an object", i)
		}
		required := []string{"type", "trigger", "content", "confidence", "scope"}
		for _, key := range required {
			if _, exists := itemObj[key]; !exists {
				return fmt.Errorf("schema: experiences[%d] missing required property %q", i, key)
			}
		}
		for k := range itemObj {
			switch k {
			case "type", "trigger", "content", "confidence", "scope", "scope_key":
			default:
				return fmt.Errorf("schema: experiences[%d] unexpected property %q", i, k)
			}
		}
		if err := assertStringEnum(itemObj, "type", []string{
			"EPISODIC", "SEMANTIC", "PROCEDURAL", "FAILURE", "CONSTRAINT", "PREFERENCE",
		}, i); err != nil {
			return err
		}
		if err := assertNonEmptyString(itemObj, "trigger", i); err != nil {
			return err
		}
		if err := assertNonEmptyString(itemObj, "content", i); err != nil {
			return err
		}
		if err := assertStringEnum(itemObj, "scope", []string{
			"GLOBAL", "TENANT", "TEAM", "USER", "AGENT", "TOOL", "TASK_TYPE",
		}, i); err != nil {
			return err
		}
		if err := assertConfidence(itemObj, i); err != nil {
			return err
		}
		if v, ok := itemObj["scope_key"]; ok {
			if _, isStr := v.(string); !isStr {
				return fmt.Errorf("schema: experiences[%d].scope_key must be a string", i)
			}
		}
	}
	return nil
}

func assertStringEnum(obj map[string]any, key string, allowed []string, idx int) error {
	v, ok := obj[key].(string)
	if !ok {
		return fmt.Errorf("schema: experiences[%d].%s must be a string", idx, key)
	}
	for _, a := range allowed {
		if v == a {
			return nil
		}
	}
	return fmt.Errorf("schema: experiences[%d].%s value %q is not allowed", idx, key, v)
}

func assertNonEmptyString(obj map[string]any, key string, idx int) error {
	v, ok := obj[key].(string)
	if !ok {
		return fmt.Errorf("schema: experiences[%d].%s must be a string", idx, key)
	}
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("schema: experiences[%d].%s must be non-empty", idx, key)
	}
	return nil
}

func assertConfidence(obj map[string]any, idx int) error {
	switch v := obj["confidence"].(type) {
	case float64:
		if v < 0 || v > 1 {
			return fmt.Errorf("schema: experiences[%d].confidence must be in [0,1]", idx)
		}
		return nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return fmt.Errorf("schema: experiences[%d].confidence invalid number", idx)
		}
		if f < 0 || f > 1 {
			return fmt.Errorf("schema: experiences[%d].confidence must be in [0,1]", idx)
		}
		return nil
	default:
		return fmt.Errorf("schema: experiences[%d].confidence must be a number", idx)
	}
}
