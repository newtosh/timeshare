package onepassword

import "testing"

func TestSecretReferenceEscapesItemPunctuation(t *testing.T) {
	got := SecretReference("my-vault", "Team/Prod DB", "password")
	want := "op://my-vault/Team%2FProd%20DB/password"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSecretReferenceDefaultField(t *testing.T) {
	got := SecretReference("v", "item", "")
	want := "op://v/item/password"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSecretRefSegment(t *testing.T) {
	if got := secretRefSegment("a/b"); got != "a%2Fb" {
		t.Fatalf("slash: got %q", got)
	}
	if got := secretRefSegment("plain"); got != "plain" {
		t.Fatalf("plain: got %q", got)
	}
}
