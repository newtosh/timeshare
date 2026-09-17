package cli

import (
	"strings"
	"testing"
)

func TestRenderStepBlockShowsDoneStepsWithValue(t *testing.T) {
	steps := []wizardStep{
		{Label: "Vault name", Value: "project-x-secrets", Done: true},
		{Label: "Auth mode", Value: "", Done: false},
	}
	out := renderStepBlock(steps, 1, "Auth mode:")

	if !strings.Contains(out, "Vault name") || !strings.Contains(out, "project-x-secrets") {
		t.Fatalf("expected completed step label and value in output, got:\n%s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("expected a checkmark for the completed step, got:\n%s", out)
	}
}

func TestRenderStepBlockShowsActiveContent(t *testing.T) {
	steps := []wizardStep{{Label: "Vault name", Done: false}}
	out := renderStepBlock(steps, 0, "Vault name:\nproject-x-secrets_")

	if !strings.Contains(out, "project-x-secrets_") {
		t.Fatalf("expected active step content in output, got:\n%s", out)
	}
}

func TestRenderStepBlockMarksActiveStepDistinctly(t *testing.T) {
	steps := []wizardStep{
		{Label: "Vault name", Value: "x", Done: true},
		{Label: "Items", Done: false},
		{Label: "TTL", Done: false},
	}
	out := renderStepBlock(steps, 1, "Items:")

	if !strings.Contains(out, "▸") {
		t.Fatalf("expected an active-step marker (▸), got:\n%s", out)
	}
}
