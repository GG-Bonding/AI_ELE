package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ApplyFeedback creates/retrieves a learning event and applies it via LearningApplier.
// Duplicate feedback for the same version is exactly-once: APPLIED events are no-ops;
// PENDING/FAILED events are re-applied through the applier (never silently dropped).
func ApplyFeedback(
	ctx context.Context,
	repo Repository,
	learningStore LearningStore,
	applier LearningApplier,
	tenantID, feedbackID, versionID, executionID string,
	reward, confidence, credit float64,
) error {
	if repo == nil || learningStore == nil {
		return fmt.Errorf("%w: repo and learningStore are required", ErrInvalidInput)
	}
	if applier == nil {
		applier = NewMemoryLearningApplier(repo, learningStore)
	}
	tenantID = strings.TrimSpace(tenantID)
	feedbackID = strings.TrimSpace(feedbackID)
	versionID = strings.TrimSpace(versionID)
	if tenantID == "" || feedbackID == "" || versionID == "" {
		return fmt.Errorf("%w: tenant_id, feedback_id, and version_id are required", ErrInvalidInput)
	}
	if credit == 0 {
		credit = 1
	}

	existing, err := learningStore.GetLearningEventByFeedbackVersion(ctx, tenantID, feedbackID, versionID)
	if err == nil && existing.ID != "" {
		if existing.Status == "APPLIED" {
			return nil
		}
		_, applyErr := applier.ApplyPending(ctx, tenantID, existing.ID)
		return applyErr
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
		if errors.Is(err, ErrDuplicateLearning) || errors.Is(err, ErrConflict) {
			again, getErr := learningStore.GetLearningEventByFeedbackVersion(ctx, tenantID, feedbackID, versionID)
			if getErr != nil {
				return getErr
			}
			if again.Status == "APPLIED" {
				return nil
			}
			_, applyErr := applier.ApplyPending(ctx, tenantID, again.ID)
			return applyErr
		}
		return err
	}

	_, err = applier.ApplyPending(ctx, tenantID, created.ID)
	return err
}
