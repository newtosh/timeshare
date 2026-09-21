package cli

import (
	"strings"
	"testing"
)

func TestRenderBreadcrumbShowsDoneStepsWithValue(t *testing.T) {
	steps := []wizardStep{
		{Label: "Repo vault name", Value: "project-x-secrets", Done: true},
		{Label: "Auth mode", Value: "", Done: false},
	}
	out := renderBreadcrumb(steps, 1)

	if !strings.Contains(out, "Repo vault name") || !strings.Contains(out, "project-x-secrets") {
		t.Fatalf("expected completed step label and value in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Auth mode") {
		t.Fatalf("expected the active step's label in output, got:\n%s", out)
	}
}

func TestRenderBreadcrumbJoinsStepsWithSeparator(t *testing.T) {
	steps := []wizardStep{
		{Label: "Repo vault name", Done: true, Value: "x"},
		{Label: "Items to copy", Done: false},
		{Label: "TTL", Done: false},
	}
	out := renderBreadcrumb(steps, 1)

	if !strings.Contains(out, ">") {
		t.Fatalf("expected a separator between steps, got:\n%s", out)
	}
	if !strings.Contains(out, "TTL") {
		t.Fatalf("expected a pending step's label in output, got:\n%s", out)
	}
}

func TestRenderBreadcrumbOmitsValueForUndoneSteps(t *testing.T) {
	steps := []wizardStep{{Label: "Repo vault name", Value: "", Done: false}}
	out := renderBreadcrumb(steps, 0)

	if strings.Count(out, "Repo vault name") != 1 {
		t.Fatalf("expected the label exactly once with no appended value, got:\n%s", out)
	}
}
