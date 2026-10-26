package testutil

import (
	"fmt"

	"github.com/google/uuid"
)

// ID gives fixtures readable, deterministic UUIDs without assuming generated IDs.
func ID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}
