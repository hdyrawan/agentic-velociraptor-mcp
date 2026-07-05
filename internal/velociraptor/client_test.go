package velociraptor

import (
	"context"
	"testing"
)

func TestNewClientSatisfiesInterfaceAndFailsClosed(t *testing.T) {
	var c Client = NewClient()

	if _, err := c.HealthCheck(context.Background()); err != ErrNotImplemented {
		t.Fatalf("HealthCheck: expected ErrNotImplemented, got %v", err)
	}
}
