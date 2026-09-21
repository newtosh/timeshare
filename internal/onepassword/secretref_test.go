package onepassword

import "testing"

func TestSecretReferenceEscapesItemPunctuation(t *testing.T) {
	got := SecretReference("my-vault", "Team/Prod DB", "password")
	want := "op://my-vault/Team%2FProd%20DB/password"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
