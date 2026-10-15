package onepassword

import "testing"

func TestRedactOpArgsHidesSecretAssignments(t *testing.T) {
	got := redactOpArgs([]string{
		"item", "create",
		"--title=DATABASE_URL",
		"password=super-secret-value",
		"username=ci",
		"token=another-secret",
	})
	want := "item create --title=DATABASE_URL password=<redacted> username=ci token=<redacted>"
	if got != want {
		t.Fatalf("got %q", got)
	}
}
