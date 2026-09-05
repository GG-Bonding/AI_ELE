package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/skill"
)

// SkillAssetRepository persists V3 Skill + SkillVersion rows.
type SkillAssetRepository struct {
	db *sql.DB
}

// NewSkillAssetRepository constructs a Postgres-backed V3 skill repository.
func NewSkillAssetRepository(db *sql.DB) *SkillAssetRepository {
	return &SkillAssetRepository{db: db}
}

func (r *SkillAssetRepository) CreateSkill(ctx context.Context, sk skill.Skill) (skill.Skill, error) {
	var active any
	if sk.ActiveVersionID != nil && strings.TrimSpace(*sk.ActiveVersionID) != "" {
		active = strings.TrimSpace(*sk.ActiveVersionID)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO skills (
			id, tenant_id, name, description, status, active_version_id, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, sk.ID, sk.TenantID, sk.Name, sk.Description, string(sk.Status), active, sk.CreatedAt, sk.UpdatedAt)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("insert skill: %w", err)
	}
	return sk, nil
}

func (r *SkillAssetRepository) GetSkill(ctx context.Context, tenantID, id string) (skill.Skill, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, description, status, active_version_id, created_at, updated_at
		FROM skills WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	sk, err := scanSkillAsset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Skill{}, skill.ErrNotFound
		}
		return skill.Skill{}, fmt.Errorf("get skill: %w", err)
	}
	return sk, nil
}

func (r *SkillAssetRepository) GetSkillByName(ctx context.Context, tenantID, name string) (skill.Skill, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, description, status, active_version_id, created_at, updated_at
		FROM skills WHERE tenant_id = $1 AND name = $2
	`, tenantID, name)
	sk, err := scanSkillAsset(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Skill{}, skill.ErrNotFound
		}
		return skill.Skill{}, fmt.Errorf("get skill by name: %w", err)
	}
	return sk, nil
}

func (r *SkillAssetRepository) UpdateSkill(ctx context.Context, sk skill.Skill) (skill.Skill, error) {
	var active any
	if sk.ActiveVersionID != nil && strings.TrimSpace(*sk.ActiveVersionID) != "" {
		active = strings.TrimSpace(*sk.ActiveVersionID)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE skills
		SET name = $3, description = $4, status = $5, active_version_id = $6, updated_at = $7
		WHERE tenant_id = $1 AND id = $2
	`, sk.TenantID, sk.ID, sk.Name, sk.Description, string(sk.Status), active, sk.UpdatedAt)
	if err != nil {
		return skill.Skill{}, fmt.Errorf("update skill: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.Skill{}, skill.ErrNotFound
	}
	return sk, nil
}

func (r *SkillAssetRepository) CreateVersion(ctx context.Context, ver skill.Version) (skill.Version, error) {
	specJSON, err := json.Marshal(ver.Spec)
	if err != nil {
		return skill.Version{}, fmt.Errorf("marshal spec: %w", err)
	}
	var pattern any
	if strings.TrimSpace(ver.PatternID) != "" {
		pattern = ver.PatternID
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO skill_versions (
			id, skill_id, tenant_id, version, pattern_id,
			spec_json, spec_yaml, spec_hash, confidence, utility,
			status, validation_status, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`,
		ver.ID, ver.SkillID, ver.TenantID, ver.Version, pattern,
		specJSON, ver.SpecYAML, ver.SpecHash, ver.Confidence, ver.Utility,
		string(ver.Status), string(ver.ValidationStatus), ver.CreatedAt,
	)
	if err != nil {
		return skill.Version{}, fmt.Errorf("insert skill version: %w", err)
	}
	return ver, nil
}

func (r *SkillAssetRepository) GetVersion(ctx context.Context, tenantID, id string) (skill.Version, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, skill_id, tenant_id, version, COALESCE(pattern_id, ''),
		       spec_json, spec_yaml, spec_hash, confidence, utility,
		       status, validation_status, created_at
		FROM skill_versions WHERE tenant_id = $1 AND id = $2
	`, tenantID, id)
	ver, err := scanSkillVersion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Version{}, skill.ErrNotFound
		}
		return skill.Version{}, fmt.Errorf("get skill version: %w", err)
	}
	return ver, nil
}

func (r *SkillAssetRepository) ListVersions(ctx context.Context, tenantID, skillID string) ([]skill.Version, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, skill_id, tenant_id, version, COALESCE(pattern_id, ''),
		       spec_json, spec_yaml, spec_hash, confidence, utility,
		       status, validation_status, created_at
		FROM skill_versions
		WHERE tenant_id = $1 AND skill_id = $2
		ORDER BY version ASC
	`, tenantID, skillID)
	if err != nil {
		return nil, fmt.Errorf("list skill versions: %w", err)
	}
	defer rows.Close()
	var out []skill.Version
	for rows.Next() {
		ver, err := scanSkillVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ver)
	}
	return out, rows.Err()
}

func (r *SkillAssetRepository) GetVersionByNumber(ctx context.Context, tenantID, skillID string, version int64) (skill.Version, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, skill_id, tenant_id, version, COALESCE(pattern_id, ''),
		       spec_json, spec_yaml, spec_hash, confidence, utility,
		       status, validation_status, created_at
		FROM skill_versions
		WHERE tenant_id = $1 AND skill_id = $2 AND version = $3
	`, tenantID, skillID, version)
	ver, err := scanSkillVersion(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return skill.Version{}, skill.ErrNotFound
		}
		return skill.Version{}, fmt.Errorf("get skill version by number: %w", err)
	}
	return ver, nil
}

type skillAssetScanner interface {
	Scan(dest ...any) error
}

func scanSkillAsset(row skillAssetScanner) (skill.Skill, error) {
	var sk skill.Skill
	var status string
	var active sql.NullString
	var createdAt, updatedAt time.Time
	if err := row.Scan(
		&sk.ID, &sk.TenantID, &sk.Name, &sk.Description, &status, &active, &createdAt, &updatedAt,
	); err != nil {
		return skill.Skill{}, err
	}
	sk.Status = skill.Status(status)
	if active.Valid && active.String != "" {
		v := active.String
		sk.ActiveVersionID = &v
	}
	sk.CreatedAt = createdAt
	sk.UpdatedAt = updatedAt
	return sk, nil
}

func scanSkillVersion(row skillAssetScanner) (skill.Version, error) {
	var ver skill.Version
	var status, validation string
	var specJSON []byte
	var createdAt time.Time
	if err := row.Scan(
		&ver.ID, &ver.SkillID, &ver.TenantID, &ver.Version, &ver.PatternID,
		&specJSON, &ver.SpecYAML, &ver.SpecHash, &ver.Confidence, &ver.Utility,
		&status, &validation, &createdAt,
	); err != nil {
		return skill.Version{}, err
	}
	if err := json.Unmarshal(specJSON, &ver.Spec); err != nil {
		return skill.Version{}, fmt.Errorf("decode spec_json: %w", err)
	}
	ver.Status = skill.VersionStatus(status)
	ver.ValidationStatus = skill.ValidationStatus(validation)
	ver.CreatedAt = createdAt
	return ver, nil
}
