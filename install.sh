#!/bin/sh
# Installs timeshare and timesharedd via `go install`, fixes up PATH if
# needed, and checks for the 1Password CLI. Safe to re-run.
set -eu

MODULE="github.com/newtosh/timeshare"
MIN_GO_VERSION="1.27.1"

# Color variables default to empty (plain text) and are only populated
# with real ANSI codes when stdout is a real terminal — same convention
# used by bun/starship/rustup's own installers. Without this, piping
# install.sh's output to a file or CI log captures raw escape-code
# garbage instead of degrading to plain text.
Blue='' Cyan='' Yellow='' Red='' Reset=''
if [ -t 1 ]; then
	Blue='\033[1;34m'
	Cyan='\033[1;36m'
	Yellow='\033[1;33m'
	Red='\033[1;31m'
	Reset='\033[0m'
fi

# %b (not embedding the color vars directly in the format string) for two
# reasons: it expands the \033 escape within an argument the way %s never
# would, and it keeps the format string a fixed literal that never starts
# with the color var's expansion — when colors are disabled (empty), a
# format string starting with "->" gets parsed by printf as an option flag
# ("invalid option"), not literal text. %b sidesteps both problems.
info()  { printf '%b%s%b %s\n' "$Blue" "==>" "$Reset" "$1"; }
hint()  { printf '%b%s%b %s\n' "$Cyan" "->" "$Reset" "$1"; }
warn()  { printf '%b%s%b %s\n' "$Yellow" "!!" "$Reset" "$1"; }
fail()  { printf '%b%s%b %s\n' "$Red" "xx" "$Reset" "$1" >&2; exit 1; }

# with_spinner LABEL CMD...: runs CMD in the background with a spinner next
# to LABEL while stdout is a real terminal; otherwise just prints LABEL and
# runs CMD in the foreground (piped output, CI — no animation to corrupt).
# CMD's own stdout/stderr is buffered to a temp file and flushed after the
# spinner line finishes, so its output can't interleave with the spinner's
# carriage-return redraws. Propagates CMD's exit status. Never use this for
# a command that might prompt for input (e.g. sudo) — buffering would eat
# the prompt along with everything else.
with_spinner() {
	label=$1
	shift
	if [ ! -t 1 ]; then
		info "$label"
		"$@"
		return $?
	fi

	log=$(mktemp "${TMPDIR:-/tmp}/timeshare-install.XXXXXX")
	"$@" >"$log" 2>&1 &
	cmd_pid=$!
	i=0
	while kill -0 "$cmd_pid" 2>/dev/null; do
		case $((i % 4)) in
		0) frame='|' ;; 1) frame='/' ;; 2) frame='-' ;; *) frame='\' ;;
		esac
		printf '\r%b%s%b %s %s' "$Blue" "==>" "$Reset" "$label" "$frame"
		i=$((i + 1))
		sleep 0.15
	done
	status=0
	wait "$cmd_pid" || status=$?
	printf '\r%b%s%b %s   \n' "$Blue" "==>" "$Reset" "$label"
	[ -s "$log" ] && cat "$log"
	rm -f "$log"
	return "$status"
}

# version_ge A B: true if version A >= B (dotted numeric versions).
version_ge() {
	[ "$1" = "$2" ] && return 0
	highest=$(printf '%s\n%s\n' "$1" "$2" | sort -t. -k1,1n -k2,2n -k3,3n | tail -n1)
	[ "$highest" = "$1" ]
}

command -v go >/dev/null 2>&1 || fail "go is not installed. Install it from https://go.dev/dl/ and re-run this script."

# upgrade_cmd_for_go: prints the upgrade command for whichever package
# manager owns the current `go` binary, or nothing if none is detected.
upgrade_cmd_for_go() {
	if command -v brew >/dev/null 2>&1 && brew list go >/dev/null 2>&1; then
		echo "brew upgrade go"
	elif command -v pacman >/dev/null 2>&1 && pacman -Qi go >/dev/null 2>&1; then
		echo "sudo pacman -Syu go"
	elif command -v apt-get >/dev/null 2>&1 && dpkg -l golang-go >/dev/null 2>&1; then
		echo "sudo apt-get update && sudo apt-get install --only-upgrade golang-go"
	fi
}

GO_VERSION=$(go version | sed -n 's/^go version go\([0-9.]*\).*/\1/p')
if [ -z "$GO_VERSION" ]; then
	warn "couldn't parse 'go version' output — continuing anyway, go install will fail loudly if the version is too old."
elif ! version_ge "$GO_VERSION" "$MIN_GO_VERSION"; then
	still_stale="go $GO_VERSION found, but timeshare requires go >= $MIN_GO_VERSION. Update from https://go.dev/dl/ and re-run."
	upgrade_cmd=$(upgrade_cmd_for_go)
	if [ -n "$upgrade_cmd" ] && [ -r /dev/tty ]; then
		warn "go $GO_VERSION found, but timeshare requires go >= $MIN_GO_VERSION."
		printf 'Run this now? %s [y/N] ' "$upgrade_cmd"
		read -r reply </dev/tty
		case "$reply" in
		[Yy]*)
			case "$upgrade_cmd" in
			*sudo*) eval "$upgrade_cmd" </dev/tty ;;
			*) with_spinner "Upgrading go..." sh -c "$upgrade_cmd" ;;
			esac
			GO_VERSION=$(go version | sed -n 's/^go version go\([0-9.]*\).*/\1/p')
			version_ge "$GO_VERSION" "$MIN_GO_VERSION" && still_stale=""
			;;
		esac
	fi
	[ -n "$still_stale" ] && fail "$still_stale"
fi

# GOPRIVATE (not GOPROXY=direct): this module is untagged, so
# proxy.golang.org can cache a stale pseudo-version resolution after a new
# commit lands. GOPRIVATE skips both the proxy AND sum.golang.org for this
# module specifically — GOPROXY=direct alone still tries to verify against
# sum.golang.org, which 404s for a version the proxy never saw and falls
# back to a second network round-trip (`git ls-remote` against github.com
# directly), a real failure point on networks that restrict direct GitHub
# access. GOPRIVATE avoids that entirely without disabling sumdb globally.
with_spinner "Installing timeshare and timesharedd..." env GOPRIVATE="$MODULE" go install "${MODULE}/cmd/timeshare@latest" "${MODULE}/cmd/timesharedd@latest"

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
			hint "Run 'source $rc_file' or open a new terminal before using timeshare."
		fi
	else
		warn "Couldn't detect your shell config file."
		hint "Add this to your shell's rc file manually: export PATH=\"$GOBIN:\$PATH\""
	fi
	;;
esac

if command -v op >/dev/null 2>&1; then
	# `op whoami` checks for a CLI session token and can report "not signed
	# in" even when the desktop-app biometric integration works fine —
	# `op vault list` is what timeshare's own init wizard actually calls,
	# so it's the real signal for whether things will work.
	if op vault list >/dev/null 2>&1; then
		info "1Password CLI (op) found and working."
	else
		warn "1Password CLI (op) found but couldn't list vaults."
		hint "Run: op signin"
	fi
else
	warn "1Password CLI (op) not found on PATH."
	hint "Install it: https://developer.1password.com/docs/cli/get-started/"
fi

info "Done. Get started in a git repo:"
hint "timeshare init"
info "See https://github.com/newtosh/timeshare#readme for the full walkthrough."
