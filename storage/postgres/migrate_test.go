package postgres_test

import (
	"testing"

	"github.com/agent-experience-engine/agent-experience-engine/storage/postgres"
)

func TestMigrateNilDBFailsFast(t *testing.T) {
	t.Parallel()

	err := postgres.Migrate(nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}
