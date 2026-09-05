package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProposeRevision creates the next immutable CANDIDATE version from new YAML.
// Prior versions are never mutated.
func ProposeRevision(
	ctx context.Context,
	repo Repository,
	tenantID, skillID, newYAML, patternID string,
) (Version, error) {
	if repo == nil {
		return Version{}, fmt.Errorf("%w: repo is required", ErrInvalidInput)
	}
	tenantID = strings.TrimSpace(tenantID)
	skillID = strings.TrimSpace(skillID)
	if tenantID == "" || skillID == "" {
		return Version{}, fmt.Errorf("%w: tenant_id and skill_id are required", ErrInvalidInput)
	}
	if _, err := repo.GetSkill(ctx, tenantID, skillID); err != nil {
		return Version{}, err
	}

	spec, err := ParseYAML(newYAML)
	if err != nil {
		return Version{}, err
	}

	existing, err := repo.ListVersions(ctx, tenantID, skillID)
	if err != nil {
		return Version{}, err
	}
	var next int64 = 1
	var conf, util float64 = 0.5, 0.5
	for _, v := range existing {
		if v.Version >= next {
			next = v.Version + 1
		}
		if v.Status == VersionActive || v.Status == VersionShadow {
			conf, util = v.Confidence, v.Utility
		}
	}

	now := time.Now().UTC()
	ver, err := NewVersion(tenantID, skillID, patternID, next, spec, newYAML, conf, util, now)
	if err != nil {
		return Version{}, err
	}
	ver.ID = uuid.NewString()
	ver = WithSeededBeta(ver)
	return repo.CreateVersion(ctx, ver)
}
