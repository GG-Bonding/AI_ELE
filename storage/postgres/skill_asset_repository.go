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
	var emb any
	if len(ver.Embedding) > 0 {
		embStr, err := formatVector(ver.Embedding)
		if err != nil {
			return skill.Version{}, err
		}
		emb = embStr
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO skill_versions (
			id, skill_id, tenant_id, version, pattern_id,
			spec_json, spec_yaml, spec_hash, confidence, utility,
			alpha, beta, success_count, failure_count, shadow_successes, shadow_failures,
			status, validation_status, created_at, embedding
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::vector)
	`,
		ver.ID, ver.SkillID, ver.TenantID, ver.Version, pattern,
		specJSON, ver.SpecYAML, ver.SpecHash, ver.Confidence, ver.Utility,
		ver.Alpha, ver.Beta, ver.SuccessCount, ver.FailureCount, ver.ShadowSuccesses, ver.ShadowFailures,
		string(ver.Status), string(ver.ValidationStatus), ver.CreatedAt, emb,
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
		       COALESCE(alpha,1), COALESCE(beta,1), COALESCE(success_count,0), COALESCE(failure_count,0),
		       COALESCE(shadow_successes,0), COALESCE(shadow_failures,0),
		       status, validation_status, created_at, embedding::text
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
		       COALESCE(alpha,1), COALESCE(beta,1), COALESCE(success_count,0), COALESCE(failure_count,0),
		       COALESCE(shadow_successes,0), COALESCE(shadow_failures,0),
		       status, validation_status, created_at, embedding::text
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
		       COALESCE(alpha,1), COALESCE(beta,1), COALESCE(success_count,0), COALESCE(failure_count,0),
		       COALESCE(shadow_successes,0), COALESCE(shadow_failures,0),
		       status, validation_status, created_at, embedding::text
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

func (r *SkillAssetRepository) ListSkills(ctx context.Context, tenantID string, statuses []skill.Status) ([]skill.Skill, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, description, status, active_version_id, created_at, updated_at
		FROM skills WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allow := map[skill.Status]struct{}{}
	for _, s := range statuses {
		allow[s] = struct{}{}
	}
	var out []skill.Skill
	for rows.Next() {
		sk, err := scanSkillAsset(rows)
		if err != nil {
			return nil, err
		}
		if len(allow) > 0 {
			if _, ok := allow[sk.Status]; !ok {
				continue
			}
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

func (r *SkillAssetRepository) UpdateVersion(ctx context.Context, ver skill.Version) (skill.Version, error) {
	specJSON, err := json.Marshal(ver.Spec)
	if err != nil {
		return skill.Version{}, err
	}
	var emb any
	if len(ver.Embedding) > 0 {
		embStr, err := formatVector(ver.Embedding)
		if err != nil {
			return skill.Version{}, err
		}
		emb = embStr
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE skill_versions SET
			spec_json=$3, spec_yaml=$4, spec_hash=$5, confidence=$6, utility=$7,
			alpha=$8, beta=$9, success_count=$10, failure_count=$11,
			shadow_successes=$12, shadow_failures=$13,
			status=$14, validation_status=$15, embedding=$16::vector
		WHERE tenant_id=$1 AND id=$2
	`, ver.TenantID, ver.ID, specJSON, ver.SpecYAML, ver.SpecHash, ver.Confidence, ver.Utility,
		ver.Alpha, ver.Beta, ver.SuccessCount, ver.FailureCount, ver.ShadowSuccesses, ver.ShadowFailures,
		string(ver.Status), string(ver.ValidationStatus), emb)
	if err != nil {
		return skill.Version{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return skill.Version{}, skill.ErrNotFound
	}
	return ver, nil
}

func (r *SkillAssetRepository) ListActiveVersions(ctx context.Context, tenantID string) ([]skill.Version, error) {
	skills, err := r.ListSkills(ctx, tenantID, []skill.Status{skill.StatusActive})
	if err != nil {
		return nil, err
	}
	var out []skill.Version
	for _, sk := range skills {
		if sk.ActiveVersionID == nil {
			continue
		}
		ver, err := r.GetVersion(ctx, tenantID, *sk.ActiveVersionID)
		if err != nil {
			continue
		}
		out = append(out, ver)
	}
	return out, nil
}

func (r *SkillAssetRepository) SaveCompiled(ctx context.Context, sk skill.Skill, ver skill.Version) (skill.Skill, skill.Version, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return skill.Skill{}, skill.Version{}, fmt.Errorf("begin save compiled: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var active any
	if sk.ActiveVersionID != nil && strings.TrimSpace(*sk.ActiveVersionID) != "" {
		active = strings.TrimSpace(*sk.ActiveVersionID)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO skills (
			id, tenant_id, name, description, status, active_version_id, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, sk.ID, sk.TenantID, sk.Name, sk.Description, string(sk.Status), active, sk.CreatedAt, sk.UpdatedAt)
	if err != nil {
		return skill.Skill{}, skill.Version{}, fmt.Errorf("insert skill: %w", err)
	}

	specJSON, err := json.Marshal(ver.Spec)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	var pattern any
	if strings.TrimSpace(ver.PatternID) != "" {
		pattern = ver.PatternID
	}
	var emb any
	if len(ver.Embedding) > 0 {
		embStr, err := formatVector(ver.Embedding)
		if err != nil {
			return skill.Skill{}, skill.Version{}, err
		}
		emb = embStr
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO skill_versions (
			id, skill_id, tenant_id, version, pattern_id,
			spec_json, spec_yaml, spec_hash, confidence, utility,
			alpha, beta, success_count, failure_count, shadow_successes, shadow_failures,
			status, validation_status, created_at, embedding
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::vector)
	`,
		ver.ID, sk.ID, sk.TenantID, ver.Version, pattern,
		specJSON, ver.SpecYAML, ver.SpecHash, ver.Confidence, ver.Utility,
		ver.Alpha, ver.Beta, ver.SuccessCount, ver.FailureCount, ver.ShadowSuccesses, ver.ShadowFailures,
		string(ver.Status), string(ver.ValidationStatus), ver.CreatedAt, emb,
	)
	if err != nil {
		return skill.Skill{}, skill.Version{}, fmt.Errorf("insert skill version: %w", err)
	}
	ver.SkillID = sk.ID
	ver.TenantID = sk.TenantID
	if err := tx.Commit(); err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	return sk, ver, nil
}

func (r *SkillAssetRepository) TransitionToShadow(ctx context.Context, tenantID, skillID, versionID string) (skill.Skill, skill.Version, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE skill_versions SET status=$3 WHERE tenant_id=$1 AND id=$2 AND skill_id=$4
	`, tenantID, versionID, string(skill.VersionShadow), skillID)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Skill{}, skill.Version{}, skill.ErrNotFound
	}
	res, err = tx.ExecContext(ctx, `
		UPDATE skills SET status=$3, updated_at=$4 WHERE tenant_id=$1 AND id=$2
	`, tenantID, skillID, string(skill.StatusShadow), now)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Skill{}, skill.Version{}, skill.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	sk, err := r.GetSkill(ctx, tenantID, skillID)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	ver, err := r.GetVersion(ctx, tenantID, versionID)
	return sk, ver, err
}

func (r *SkillAssetRepository) ActivateVersion(ctx context.Context, tenantID, skillID, versionID, previousActiveID string) (skill.Skill, skill.Version, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if previousActiveID != "" && previousActiveID != versionID {
		_, err = tx.ExecContext(ctx, `
			UPDATE skill_versions SET status=$3 WHERE tenant_id=$1 AND id=$2
		`, tenantID, previousActiveID, string(skill.VersionDeprecated))
		if err != nil {
			return skill.Skill{}, skill.Version{}, err
		}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE skill_versions SET status=$3 WHERE tenant_id=$1 AND id=$2 AND skill_id=$4
	`, tenantID, versionID, string(skill.VersionActive), skillID)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Skill{}, skill.Version{}, skill.ErrNotFound
	}
	res, err = tx.ExecContext(ctx, `
		UPDATE skills SET status=$3, active_version_id=$4, updated_at=$5 WHERE tenant_id=$1 AND id=$2
	`, tenantID, skillID, string(skill.StatusActive), versionID, now)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Skill{}, skill.Version{}, skill.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	sk, err := r.GetSkill(ctx, tenantID, skillID)
	if err != nil {
		return skill.Skill{}, skill.Version{}, err
	}
	ver, err := r.GetVersion(ctx, tenantID, versionID)
	return sk, ver, err
}

func (r *SkillAssetRepository) SuspendActive(ctx context.Context, tenantID, skillID, versionID string) (skill.Skill, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return skill.Skill{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if versionID != "" {
		_, err = tx.ExecContext(ctx, `
			UPDATE skill_versions SET status=$3 WHERE tenant_id=$1 AND id=$2
		`, tenantID, versionID, string(skill.VersionSuspended))
		if err != nil {
			return skill.Skill{}, err
		}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE skills SET status=$3, updated_at=$4 WHERE tenant_id=$1 AND id=$2
	`, tenantID, skillID, string(skill.StatusSuspended), now)
	if err != nil {
		return skill.Skill{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Skill{}, skill.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return skill.Skill{}, err
	}
	return r.GetSkill(ctx, tenantID, skillID)
}

func (r *SkillAssetRepository) IncrementShadowOutcome(ctx context.Context, tenantID, versionID string, success bool) (skill.Version, error) {
	col := "shadow_failures"
	if success {
		col = "shadow_successes"
	}
	//nolint:gosec // col is fixed literal
	res, err := r.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE skill_versions SET %s = %s + 1 WHERE tenant_id=$1 AND id=$2
	`, col, col), tenantID, versionID)
	if err != nil {
		return skill.Version{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return skill.Version{}, skill.ErrNotFound
	}
	return r.GetVersion(ctx, tenantID, versionID)
}

func (r *SkillAssetRepository) SearchActiveByEmbedding(ctx context.Context, tenantID string, query []float32, topK int) ([]skill.ScoredVersion, error) {
	if len(query) == 0 {
		return nil, skill.ErrNotSupported
	}
	if topK <= 0 {
		topK = 20
	}
	vec, err := formatVector(query)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.tenant_id, s.name, s.description, s.status, s.active_version_id, s.created_at, s.updated_at,
		       v.id, v.skill_id, v.tenant_id, v.version, COALESCE(v.pattern_id, ''),
		       v.spec_json, v.spec_yaml, v.spec_hash, v.confidence, v.utility,
		       COALESCE(v.alpha,1), COALESCE(v.beta,1), COALESCE(v.success_count,0), COALESCE(v.failure_count,0),
		       COALESCE(v.shadow_successes,0), COALESCE(v.shadow_failures,0),
		       v.status, v.validation_status, v.created_at, v.embedding::text,
		       1 - (v.embedding <=> $2::vector) AS similarity
		FROM skills s
		JOIN skill_versions v ON v.id = s.active_version_id AND v.tenant_id = s.tenant_id
		WHERE s.tenant_id = $1
		  AND s.status = 'ACTIVE'
		  AND v.validation_status = 'PASSED'
		  AND v.embedding IS NOT NULL
		ORDER BY v.embedding <=> $2::vector
		LIMIT $3
	`, tenantID, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("search skill embeddings: %w", err)
	}
	defer rows.Close()
	var out []skill.ScoredVersion
	for rows.Next() {
		var sk skill.Skill
		var status string
		var active sql.NullString
		var createdAt, updatedAt time.Time
		var ver skill.Version
		var vStatus, validation string
		var specJSON []byte
		var vCreated time.Time
		var embText sql.NullString
		var sim float64
		if err := rows.Scan(
			&sk.ID, &sk.TenantID, &sk.Name, &sk.Description, &status, &active, &createdAt, &updatedAt,
			&ver.ID, &ver.SkillID, &ver.TenantID, &ver.Version, &ver.PatternID,
			&specJSON, &ver.SpecYAML, &ver.SpecHash, &ver.Confidence, &ver.Utility,
			&ver.Alpha, &ver.Beta, &ver.SuccessCount, &ver.FailureCount,
			&ver.ShadowSuccesses, &ver.ShadowFailures,
			&vStatus, &validation, &vCreated, &embText, &sim,
		); err != nil {
			return nil, err
		}
		sk.Status = skill.Status(status)
		if active.Valid && active.String != "" {
			v := active.String
			sk.ActiveVersionID = &v
		}
		sk.CreatedAt = createdAt
		sk.UpdatedAt = updatedAt
		if err := json.Unmarshal(specJSON, &ver.Spec); err != nil {
			return nil, err
		}
		ver.Status = skill.VersionStatus(vStatus)
		ver.ValidationStatus = skill.ValidationStatus(validation)
		ver.CreatedAt = vCreated
		if embText.Valid && strings.TrimSpace(embText.String) != "" {
			emb, err := parseVector(embText.String)
			if err != nil {
				return nil, err
			}
			ver.Embedding = emb
		}
		out = append(out, skill.ScoredVersion{Skill: sk, Version: ver, Similarity: sim})
	}
	return out, rows.Err()
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
	var embText sql.NullString
	if err := row.Scan(
		&ver.ID, &ver.SkillID, &ver.TenantID, &ver.Version, &ver.PatternID,
		&specJSON, &ver.SpecYAML, &ver.SpecHash, &ver.Confidence, &ver.Utility,
		&ver.Alpha, &ver.Beta, &ver.SuccessCount, &ver.FailureCount,
		&ver.ShadowSuccesses, &ver.ShadowFailures,
		&status, &validation, &createdAt, &embText,
	); err != nil {
		return skill.Version{}, err
	}
	if err := json.Unmarshal(specJSON, &ver.Spec); err != nil {
		return skill.Version{}, fmt.Errorf("decode spec_json: %w", err)
	}
	ver.Status = skill.VersionStatus(status)
	ver.ValidationStatus = skill.ValidationStatus(validation)
	ver.CreatedAt = createdAt
	if embText.Valid && strings.TrimSpace(embText.String) != "" {
		emb, err := parseVector(embText.String)
		if err != nil {
			return skill.Version{}, fmt.Errorf("parse skill embedding: %w", err)
		}
		ver.Embedding = emb
	}
	return ver, nil
}
