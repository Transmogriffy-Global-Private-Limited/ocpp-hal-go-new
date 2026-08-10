package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/ocpp-hal-go-new/internal/config"
)

func TestChooseV1StoreFailsBeforeMemoryFallbackWhenSecretMissing(t *testing.T) {
	_, err := chooseV1Store(context.Background(), config.Config{})
	if err == nil || !strings.Contains(err.Error(), "HAL_V1_CMS_BEARER_TOKEN") {
		t.Fatalf("error=%v", err)
	}
}
