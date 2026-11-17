package handler

import (
	"fmt"
	"srez/internal/entities"
	"testing"
)

func TestManagementInvalidFieldLoggingIsAllowlisted(t *testing.T) {
	if got := managementInvalidField(fmt.Errorf("time: %w", entities.ErrInvalidValue)); got != "time" {
		t.Fatal("missing safe diagnostic")
	}
	for _, v := range []string{"https://example.test/private-token", "secret\nbrand.id", "untrusted"} {
		if got := managementInvalidField(fmt.Errorf("%s: %w", v, entities.ErrInvalidValue)); got != "unspecified" {
			t.Fatal("untrusted content entered log classification")
		}
	}
}
