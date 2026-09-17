#!/bin/sh
# Installs timeshare and timesharedd via `go install`, fixes up PATH if
# needed, and checks for the 1Password CLI. Safe to re-run.
set -eu

MODULE="github.com/newtosh/timeshare"
MIN_GO_VERSION="1.27.1"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn()  { printf '\033[1;33m!!\033[0m %s\n' "$1"; }
fail()  { printf '\033[1;31mxx\033[0m %s\n' "$1" >&2; exit 1; }

# version_ge A B: true if version A >= B (dotted numeric versions).
version_ge() {
	[ "$1" = "$2" ] && return 0
	highest=$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | tail -n1)
	[ "$highest" = "$1" ]
}

command -v go >/dev/null 2>&1 || fail "go is not installed. Install it from https://go.dev/dl/ and re-run this script."

GO_VERSION=$(go version | sed -n 's/^go version go\([0-9.]*\).*/\1/p')
if [ -z "$GO_VERSION" ]; then
	warn "couldn't parse 'go version' output — continuing anyway, go install will fail loudly if the version is too old."
elif ! version_ge "$GO_VERSION" "$MIN_GO_VERSION"; then
	fail "go $GO_VERSION found, but timeshare requires go >= $MIN_GO_VERSION. Update from https://go.dev/dl/ and re-run."
fi

info "Installing timeshare and timesharedd..."
go install "${MODULE}/cmd/timeshare@latest" "${MODULE}/cmd/timesharedd@latest"

GOBIN=$(go env GOPATH)/bin

case ":$PATH:" in
*":$GOBIN:"*)
	info "$GOBIN is already on PATH."
	;;
*)
	warn "$GOBIN is not on your PATH."
	shell_name=$(basename "${SHELL:-sh}")
	case "$shell_name" in
	zsh) rc_file="$HOME/.zshrc" ;;
	bash) rc_file="$HOME/.bashrc" ;;
	fish) rc_file="$HOME/.config/fish/config.fish" ;;
	*) rc_file="" ;;
	esac

	if [ -n "$rc_file" ]; then
		export_line="export PATH=\"$GOBIN:\$PATH\""
		if [ "$shell_name" = "fish" ]; then
			export_line="set -gx PATH $GOBIN \$PATH"
		fi

		if [ -f "$rc_file" ] && grep -qF "$GOBIN" "$rc_file" 2>/dev/null; then
			info "$rc_file already references $GOBIN — leaving it as-is."
		else
			printf '\n# added by timeshare install.sh\n%s\n' "$export_line" >>"$rc_file"
			info "Added $GOBIN to PATH in $rc_file."
			warn "Run 'source $rc_file' or open a new terminal before using timeshare."
		fi
	else
		warn "Couldn't detect your shell config file. Add this to your shell's rc file manually:"
		printf '  export PATH="%s:$PATH"\n' "$GOBIN"
	fi
	;;
esac

if command -v op >/dev/null 2>&1; then
	if op whoami >/dev/null 2>&1; then
		info "1Password CLI (op) found and signed in."
	else
		warn "1Password CLI (op) found but not signed in. Run: op signin"
	fi
else
	warn "1Password CLI (op) not found on PATH. Install it: https://developer.1password.com/docs/cli/get-started/"
fi

info "Done. Get started in a git repo:"
printf '\n  timeshare init --vault=<name> --mode=biometric --move-item=<existing-vault>\n\n'
info "See https://github.com/newtosh/timeshare#readme for the full walkthrough."
