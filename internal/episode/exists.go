package episode

import (
	"context"
	"errors"
)

// EpisodeExists reports whether an episode exists for the tenant.
func (s *Service) EpisodeExists(ctx context.Context, tenantID, episodeID string) (bool, error) {
	_, err := s.GetEpisode(ctx, tenantID, episodeID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}
