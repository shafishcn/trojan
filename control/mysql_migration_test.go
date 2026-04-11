package control

import (
	"slices"
	"testing"
)

func TestPendingControlMigrationVersions(t *testing.T) {
	applied := map[int]bool{
		1: true,
		3: true,
	}

	pending := pendingControlMigrationVersions(applied)
	want := []int{2, 4, 5, 6, 7}
	if !slices.Equal(pending, want) {
		t.Fatalf("pendingControlMigrationVersions() = %v, want %v", pending, want)
	}
}

func TestControlMigrationsOrdered(t *testing.T) {
	if len(controlMigrations) == 0 {
		t.Fatal("controlMigrations should not be empty")
	}
	lastVersion := 0
	for _, migration := range controlMigrations {
		if migration.Version <= lastVersion {
			t.Fatalf("migration version %d is not strictly increasing after %d", migration.Version, lastVersion)
		}
		if migration.Name == "" {
			t.Fatalf("migration version %d has empty name", migration.Version)
		}
		if migration.Apply == nil {
			t.Fatalf("migration version %d has nil Apply", migration.Version)
		}
		lastVersion = migration.Version
	}
}
