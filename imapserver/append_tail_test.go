package imapserver_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

func startAppendTailServer(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Serve(ln) //nolint:errcheck
	return ln.Addr().String()
}

// TestAppendMalformedTailKeepsTheNextCommand: a client that omits the CRLF
// terminating its APPEND command line gets its message stored and an OK, and
// the framing fault is logged. What it must NOT lose is the NEXT command,
// which it sent correctly.
//
// ExpectCRLF consumes nothing when it fails, so the decoder is parked at the
// first byte of that next command; the caller's DiscardLine used to eat it
// whole, leaving the client waiting for a tag that never came (#1370).
func TestAppendMalformedTailKeepsTheNextCommand(t *testing.T) {
	addr := startAppendTailServer(t)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	rd := bufio.NewReader(c)
	read := func(what string) string {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, err := rd.ReadString('\n')
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return strings.TrimRight(line, "\r\n")
	}
	until := func(tag, what string) {
		for {
			if strings.HasPrefix(read(what), tag+" ") {
				return
			}
		}
	}
	read("greeting")
	fmt.Fprint(c, "a1 LOGIN u p\r\n")
	until("a1", "login")

	// The literal's last CRLF belongs to the message; the command line is left
	// unterminated, and the next command follows immediately.
	body := "From: a@b\r\nSubject: t\r\n\r\nhello\r\n"
	fmt.Fprintf(c, "a2 APPEND INBOX {%d+}\r\n%s", len(body), body)
	fmt.Fprint(c, "a3 NOOP\r\n")

	until("a2", "append")
	if got := read("noop after a malformed append tail"); !strings.HasPrefix(got, "a3 ") {
		t.Fatalf("the next command was discarded by the resync; got %q", got)
	}
}
