package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/config"
	"github.com/Transmogriffy-Global-Private-Limited/OCPPHAL_Go/internal/store"
)

func TestChooseV1StoreFailsBeforeMemoryFallbackWhenSecretMissing(t *testing.T) {
	_, err := chooseV1Store(context.Background(), config.Config{V1Enabled: true}, store.NewMemoryStore())
	if err == nil || !strings.Contains(err.Error(), "HAL_V1_CMS_BEARER_TOKEN") {
		t.Fatalf("error=%v", err)
	}
}
