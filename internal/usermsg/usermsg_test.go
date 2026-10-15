package usermsg

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		err  error
		want Kind
	}{
		{nil, KindUnknown},
		{errors.New("authorization prompt dismissed, please try again"), KindPromptDismissed},
		{errors.New("1Password: agent returned an error"), KindAgent},
		{fmt.Errorf("dial unix /tmp/agent.sock: connect: %w", syscall.ECONNREFUSED), KindUnreachable},
		{errors.New("connect: no such file or directory"), KindUnreachable},
		{errors.New("not in the allow-list"), KindUnknown},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("Classify(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestAnnotateNamesTheCaseAndRetry(t *testing.T) {
	err := Annotate(errors.New("error initializing client: authorization prompt dismissed"))
	if !strings.Contains(err.Error(), "prompt was dismissed") || !strings.Contains(err.Error(), "Retry is safe") {
		t.Fatalf("dismissed: %v", err)
	}
	err = Annotate(errors.New("agent returned an error"))
	if !strings.Contains(err.Error(), "agent returned an error") || !strings.Contains(err.Error(), "Retry is safe") {
		t.Fatalf("agent: %v", err)
	}
	err = Annotate(errors.New("connect: no such file or directory"))
	if !strings.Contains(err.Error(), "unreachable") || !strings.Contains(err.Error(), "Retry is safe") {
		t.Fatalf("unreachable: %v", err)
	}
	plain := errors.New("secret is not in this project's allow-list")
	if got := Annotate(plain); got.Error() != plain.Error() {
		t.Fatalf("unknown errors must keep their original text, got %v", got)
	}
}

func TestAnnotateRetriedMentionsTheRetry(t *testing.T) {
	err := AnnotateRetried(errors.New("agent returned an error"))
	msg := err.Error()
	for _, want := range []string{"retried the sign once", "agent returned an error", "Retry is safe"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in %q", want, msg)
		}
	}
}
