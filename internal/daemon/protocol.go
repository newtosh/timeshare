package daemon

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"timeshare/internal/config"
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
}

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

var (
	decoderMu sync.Mutex
	decoders  = make(map[io.Reader]*json.Decoder)
)

// ReadMessage decodes one JSON line into v. It caches decoders per reader
// to correctly handle multiple messages on a single stream.
func ReadMessage(r io.Reader, v any) error {
	decoderMu.Lock()
	dec, ok := decoders[r]
	if !ok {
		dec = json.NewDecoder(r)
		decoders[r] = dec
	}
	decoderMu.Unlock()

	return dec.Decode(v)
}
