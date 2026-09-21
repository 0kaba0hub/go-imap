package imapserver_test

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// The envelope a reference implementation cached for one deliberately awkward
// message: a literal subject carrying an encoded word, a quote and a
// backslash; a B-encoded display name; a quoted local part; and an empty
// address group. None of the four survives a round trip through Envelope.
const rawEnvelope = "\"Mon, 21 Sep 2026 14:05:06 +0300\" {46}\r\n" +
	"=?UTF-8?Q?Caf=C3=A9_menu?= \"quoted\" back\\slash " +
	`(("Doe, John" NIL "john" "example.test")) (("Doe, John" NIL "john" "example.test")) ((NIL NIL "reply" "example.test")) (("=?UTF-8?B?0IbQstCw0L0=?=" NIL "ivan" "example.test")(NIL NIL "quoted local" "example.test")) ((NIL NIL "undisclosed-recipients" NIL)(NIL NIL NIL NIL)) NIL "<m0@example.test>" "<m1@example.test>"`

type rawEnvelopeSession struct {
	imapserver.Session
}

func (s *rawEnvelopeSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, _ *imap.FetchOptions) error {
	mw := w.CreateMessage(1)
	mw.WriteUID(1)
	mw.WriteEnvelopeRaw(rawEnvelope)
	return mw.Close()
}

func startRawEnvelopeServer(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &rawEnvelopeSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck
	go srv.Serve(ln)                 //nolint:errcheck
	return ln.Addr().String()
}

// An envelope written as text reaches the client byte for byte: a client that
// re-encoded the subject or dropped the group would show the user something
// the message does not say.
func TestWriteEnvelopeRawReachesTheClientUnchanged(t *testing.T) {
	addr := startRawEnvelopeServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck
	rd := bufio.NewReader(c)
	read := func() string {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, rerr := rd.ReadString('\n')
		if rerr != nil {
			t.Fatalf("read: %v", rerr)
		}
		return strings.TrimRight(line, "\r\n")
	}
	read() // greeting
	for _, cmd := range []string{"a login u p", "b select INBOX"} {
		if _, werr := c.Write([]byte(cmd + "\r\n")); werr != nil {
			t.Fatal(werr)
		}
		for {
			if line := read(); strings.HasPrefix(line, cmd[:1]+" ") {
				break
			}
		}
	}
	if _, werr := c.Write([]byte("d fetch 1 (envelope)\r\n")); werr != nil {
		t.Fatal(werr)
	}
	// The subject is a literal, so the response spans lines: what matters is
	// the byte stream, not how it was cut.
	var wire strings.Builder
	for {
		line := read()
		wire.WriteString(line)
		wire.WriteString("\r\n")
		if strings.HasPrefix(line, "d ") {
			break
		}
	}
	want := "ENVELOPE (" + rawEnvelope + ")"
	if !strings.Contains(wire.String(), want) {
		t.Errorf("the client was sent\n  %q\nwhich does not carry\n  %q", wire.String(), want)
	}
}
