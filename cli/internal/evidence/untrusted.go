package evidence

// Canonical boundary markers for prompt assembly when embedding
// untrusted artifact bodies.
const (
	UntrustedDataOpen  = "<untrusted-data>"
	UntrustedDataClose = "</untrusted-data>"
)

// DelimitUntrustedData wraps body in a canonical untrusted-data envelope so
// prompt renderers can distinguish trusted instructions from artifact text.
func DelimitUntrustedData(body string) string {
	return UntrustedDataOpen + "\n" + body + "\n" + UntrustedDataClose
}
