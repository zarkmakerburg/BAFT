package ws

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func TestWSNoiseServerCancellationClosesHijackedConnection(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		name := "idle_handshake"
		if authenticated {
			name = "authenticated_read"
		}
		t.Run(name, func(t *testing.T) {
			ik, err := securityinternal.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			rk, err := securityinternal.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			streamEntered := make(chan struct{})
			h, err := HandlerWithNoise(func(ctx context.Context, in io.Reader, out io.Writer, p PeerInfo) error {
				close(streamEntered)
				_, err := io.Copy(io.Discard, in)
				return err
			}, NoiseOptions{Handshake: securityinternal.HandshakeConfig{Static: rk, PeerStatic: ik.Public}, PeerIdentity: "urn:baft:node:ir", HandshakeTimeout: 30 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				h.ServeHTTP(w, r)
			}))
			s.Config.BaseContext = func(net.Listener) context.Context { return ctx }
			s.StartTLS()
			defer s.Close()
			tr := s.Client().Transport.(*http.Transport)
			d := &Dialer{Endpoint: s.URL, TLSConfig: tr.TLSClientConfig.Clone()}
			var conn *Conn
			if authenticated {
				conn, _, err = d.OpenNoise(context.Background(), securityinternal.HandshakeConfig{Static: ik, PeerStatic: rk.Public})
			} else {
				conn, err = d.Dial(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.closeUnderlying()
			if authenticated {
				select {
				case <-streamEntered:
				case <-time.After(2 * time.Second):
					t.Fatal("authenticated stream not entered")
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("server cancellation left hijacked Noise handler blocked")
			}
		})
	}
}
