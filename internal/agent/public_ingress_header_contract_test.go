package agent

import "testing"

// A partially forged /proc header must not be treated as a complete socket
// inventory: doing so could turn an unknown port into "not_observed".
func TestPublicIngressSocketParserRejectsForgedHeaders(t *testing.T) {
	for _, header := range []string{
		"local_address\n",
		"sl local_address nonsense st\n",
		"sl local_address rem_address invalid\n",
		"sl local_address rem_address\n",
		"sl local_address rem_address st\n",
	} {
		if header == "sl local_address rem_address st\n" {
			if _, err := parseIngressSocketTable([]byte(header), false); err != nil {
				t.Fatalf("valid header rejected: %v", err)
			}
			continue
		}
		if _, err := parseIngressSocketTable([]byte(header), false); err == nil {
			t.Fatalf("forged header accepted: %q", header)
		}
	}
}
