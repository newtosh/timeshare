package daemon

import (
	"encoding/json"
	"io"
	"time"

	"github.com/newtosh/timeshare/internal/config"
)

// Request is sent by the CLI client to the daemon. The client resolves
// .timeshare.yml locally (it has filesystem access; the daemon may be
// serving requests from a different cwd) and sends the resulting policy
// alongside the request. The daemon enforces AllowedItems regardless of
// what the underlying 1Password vault grant would permit (spec: Goals).
type Request struct {
	ProjectID    string        `json:"project_id"`
	SecretName   string        `json:"secret_name"`
	Vault        string        `json:"vault"`
	Mode         config.Mode   `json:"mode"`
	TTL          time.Duration `json:"ttl"`
	AllowedItems []string      `json:"allowed_items"`
	Op           Op            `json:"op,omitempty"`
}

// Op selects which operation the daemon performs for a Request. The zero
// value (OpRead) keeps every existing Request literal from Tasks 1-11 valid
// without changes.
type Op string

const (
	OpRead   Op = "" // default/zero value keeps existing Request literals valid
	OpStatus Op = "status"
	OpLock   Op = "lock"
)

type Response struct {
	Value     string    `json:"value,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// WriteMessage encodes v as one JSON line.
func WriteMessage(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

// ReadMessage decodes one JSON line into v. It reads one byte at a time
// (no bufio.Reader) so it never consumes bytes past the trailing newline of
// its own message: a buffered reader created fresh per call would pull
// whatever is already available from r into its internal buffer, silently
// discarding any bytes beyond the first line's boundary and losing
// subsequent messages on streams that carry more than one (e.g. a
// bytes.Buffer, or a socket where the OS coalesces two writes). Reading
// stateless keeps ReadMessage safe to call any number of times on any
// io.Reader without tracking state between calls.
func ReadMessage(r io.Reader, v any) error {
	var line []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			line = append(line, b[0])
			if b[0] == '\n' {
				break
			}
		}
		if err != nil {
			if err == io.EOF && len(line) > 0 {
				break
			}
			return err
		}
	}
	return json.Unmarshal(line, v)
}
