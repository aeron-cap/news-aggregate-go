package database

import (
	"context"
	"strings"
	"testing"
)

func TestConnectionURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if got := ConnectionURL(); got != DefaultURL {
		t.Fatalf("default URL = %q", got)
	}
	const configured = "postgresql://reader@localhost:5433/custom?sslmode=disable"
	t.Setenv("DATABASE_URL", configured)
	if got := ConnectionURL(); got != configured {
		t.Fatalf("configured URL = %q", got)
	}
}

func TestOpenDoesNotExposeMalformedConnectionCredentials(t *testing.T) {
	db, err := Open(context.Background(), "postgresql://reader:private-password@localhost:invalid/news")
	if db != nil || err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatalf("unsafe configuration error: %v", err)
	}
}
