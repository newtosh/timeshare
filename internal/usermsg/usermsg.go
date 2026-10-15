// Package usermsg turns raw op and SSH-agent failures into a sentence
// that names which of the three common cases happened and whether
// running the command again is safe.
package usermsg

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
)

// Kind classifies a failure the user can act on.
type Kind int

const (
	KindUnknown Kind = iota
	KindPromptDismissed
	KindAgent
	KindUnreachable
)

const (
	dismissedMsg   = "1Password authorization prompt was dismissed. Retry is safe: approve the prompt to grant this pass."
	agentMsg       = "1Password agent returned an error. Retry is safe if the command did not finish writing its result."
	unreachableMsg = "upstream SSH agent is unreachable. Retry is safe once the agent is running."
	retriedPrefix  = "retried the sign once; still failing: "
)

// Classify reports which user-facing case err belongs to.
func Classify(err error) Kind {
	if err == nil {
		return KindUnknown
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENOENT) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return KindUnreachable
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "prompt dismissed") ||
		strings.Contains(msg, "authorization cancelled") ||
		strings.Contains(msg, "authorization canceled"):
		return KindPromptDismissed
	case strings.Contains(msg, "agent returned an error"):
		return KindAgent
	case strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "unreachable") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "i/o timeout"):
		return KindUnreachable
	default:
		return KindUnknown
	}
}

// Annotate appends a retry hint when err is one of the three known
// cases. Other errors are returned unchanged so allow-list and config
// failures keep their original text.
func Annotate(err error) error {
	if err == nil || strings.Contains(err.Error(), "Retry is safe") {
		return err
	}
	switch Classify(err) {
	case KindPromptDismissed:
		return fmt.Errorf("%s: %w", dismissedMsg, err)
	case KindAgent:
		return fmt.Errorf("%s: %w", agentMsg, err)
	case KindUnreachable:
		return fmt.Errorf("%s: %w", unreachableMsg, err)
	default:
		return err
	}
}

// AnnotateRetried is Annotate plus an explicit note that the sign was
// already attempted a second time.
func AnnotateRetried(err error) error {
	if err == nil {
		return errors.New(retriedPrefix + "signing failed")
	}
	return fmt.Errorf("%s%w", retriedPrefix, Annotate(err))
}
