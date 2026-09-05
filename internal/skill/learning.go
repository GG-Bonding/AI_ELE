package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ApplyFeedback applies one feedback reward to a SkillVersion exactly once
// (unique on feedbackID + versionID). Duplicate events are swallowed.
func ApplyFeedback(
	ctx context.Context,
	repo Repository,
	learningStore LearningStore,
	tenantID, feedbackID, versionID, executionID string,
	reward, confidence, credit float64,
) error {
	if repo == nil || learningStore == nil {
		return fmt.Errorf("%w: repo and learningStore are required", ErrInvalidInput)
	}
	tenantID = strings.TrimSpace(tenantID)
	feedbackID = strings.TrimSpace(feedbackID)
	versionID = strings.TrimSpace(versionID)
	if tenantID == "" || feedbackID == "" || versionID == "" {
		return fmt.Errorf("%w: tenant_id, feedback_id, and version_id are required", ErrInvalidInput)
	}

	existing, err := learningStore.GetLearningEventByFeedbackVersion(ctx, tenantID, feedbackID, versionID)
	if err == nil && existing.ID != "" {
		return nil
	}
	if err != nil && !errors.Is(err, ErrLearningNotFound) && !errors.Is(err, ErrNotFound) {
		return err
	}

	ver, err := repo.GetVersion(ctx, tenantID, versionID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	ev := LearningEvent{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		SkillID:        ver.SkillID,
		SkillVersionID: versionID,
		ExecutionID:    strings.TrimSpace(executionID),
		FeedbackID:     feedbackID,
		Reward:         reward,
		Confidence:     confidence,
		Credit:         credit,
		Status:         "PENDING",
		CreatedAt:      now,
	}
	created, err := learningStore.CreateLearningEvent(ctx, ev)
	if err != nil {
		if errors.Is(err, ErrDuplicateLearning) {
			return nil
		}
		return err
	}

	expReward := reward * credit
	updated, err := ApplyBetaUpdate(ver, expReward, confidence)
	if err != nil {
		_ = learningStore.MarkLearningFailed(ctx, tenantID, created.ID)
		return err
	}
	if _, err := repo.UpdateVersion(ctx, updated); err != nil {
		_ = learningStore.MarkLearningFailed(ctx, tenantID, created.ID)
		return err
	}
	return learningStore.MarkLearningApplied(ctx, tenantID, created.ID, now)
}
