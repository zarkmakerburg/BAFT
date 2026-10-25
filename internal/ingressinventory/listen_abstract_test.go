package ingressinventory

import "testing"

func TestParseListenStreamAbstractUnixSocketIsNotTCPClaimButAmbiguityStaysClosed(t *testing.T) {
	ok := "[Socket]\nListenStream=@ISCSIADM_ABSTRACT_NAMESPACE\nListenStream=54619\n"
	claims, err := parseSystemdListenStream([]byte(ok), nil, "iscsid.socket")
	if err != nil || len(claims) != 1 || claims[0].Port != 54619 {
		t.Fatalf("abstract unix socket must be ignored without hiding TCP claims: %v %+v", err, claims)
	}
	for _, bad := range []string{"@", "@80", "@a:b", "@a b", "@\"x\"", "@a/b", "https", "@a\\x"} {
		if _, err := parseSystemdListenStream([]byte("[Socket]\nListenStream="+bad+"\n"), nil, "x.socket"); err == nil {
			t.Fatalf("ambiguous ListenStream=%q accepted", bad)
		}
	}
}
