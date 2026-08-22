package luksd

const DefaultSocketPath = "/run/cadestro/luks.sock"

const userPassphraseSlot = 7

const minPassphraseLength = 16

type Request struct {
	Token      string `json:"token"`
	Passphrase string `json:"passphrase"`
}

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Code  string `json:"code,omitempty"`
}

const (
	CodeMissingToken     = "missing_token"
	CodeNotConnected     = "not_connected"
	CodeInvalidToken     = "invalid_token"
	CodePassphrasePolicy = "passphrase_policy"
	CodePassphraseReuse  = "passphrase_reuse"
	CodeKeyUnavailable   = "key_unavailable"
	CodeInternal         = "internal"
	CodeBusy             = "busy"
	CodeOK               = "ok"
)
