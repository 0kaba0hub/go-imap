package imapserver_test

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// A command the grammar refuses is answered BAD, not NO [SERVERBUG]: a parse
// failure arriving as a plain error told the client the server was broken.
func TestAMalformedCommandIsAnsweredBad(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{"sequence-set", `a2 SEARCH HEADER Subject fileinto test`},
		{"mailbox name", "a2 SELECT \"&\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := dialID(t)
			fmt.Fprint(w.conn, "a1 LOGIN u p\r\n")
			w.until("a1", "login")

			fmt.Fprint(w.conn, tc.cmd+"\r\n")
			last := ""
			for _, l := range w.until("a2", tc.name) {
				last = l
			}
			if strings.Contains(last, "SERVERBUG") {
				t.Errorf("a malformed command was answered %q: the client is told the "+
					"server is broken by its own bad syntax", last)
			}
			if !strings.HasPrefix(last, "a2 BAD") {
				t.Errorf("answer was %q, want BAD", last)
			}
		})
	}
}

// A plain error from the handler is still NO [SERVERBUG]: this classifies parse
// failures, it does not answer BAD to everything.
func TestAHandlerErrorIsStillServerBug(t *testing.T) {
	w := dialFailing(t)
	fmt.Fprint(w.conn, "a1 LOGIN u p\r\n")
	w.until("a1", "login")
	fmt.Fprint(w.conn, "a2 SELECT INBOX\r\n")
	last := ""
	for _, l := range w.until("a2", "select") {
		last = l
	}
	if !strings.Contains(last, "SERVERBUG") {
		t.Errorf("a handler error was answered %q, want NO [SERVERBUG]: parse failures and "+
			"server faults are no longer told apart", last)
	}
}

// failingSession answers a well-formed SELECT with a plain error, which is what
// a server fault looks like from the connection loop.
type failingSession struct {
	imapserver.Session
}

func (s *failingSession) Select(string, *imap.SelectOptions) (*imap.SelectData, error) {
	return nil, errors.New("boom")
}

func dialFailing(t *testing.T) *wire {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &failingSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: struct{}{}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Serve(ln) //nolint:errcheck
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	w := &wire{t: t, conn: c, rd: bufio.NewReader(c)}
	w.line("greeting")
	return w
}
