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

type idSession struct {
	imapserver.Session
}

func (s *idSession) ID(*imap.IDData) *imap.IDData {
	return &imap.IDData{Name: "test-server"}
}

func startIDServer(t *testing.T) string {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &idSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
		Caps: imap.CapSet{
			imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}, imap.CapID: struct{}{},
		},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Serve(ln) //nolint:errcheck
	return ln.Addr().String()
}

type wire struct {
	t    *testing.T
	conn net.Conn
	rd   *bufio.Reader
}

func dialID(t *testing.T) *wire {
	t.Helper()
	c, err := net.Dial("tcp", startIDServer(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	w := &wire{t: t, conn: c, rd: bufio.NewReader(c)}
	w.line("greeting")
	return w
}

func (w *wire) line(what string) string {
	w.t.Helper()
	_ = w.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	l, err := w.rd.ReadString('\n')
	if err != nil {
		w.t.Fatalf("%s: %v", what, err)
	}
	return strings.TrimRight(l, "\r\n")
}

func (w *wire) until(tag, what string) []string {
	w.t.Helper()
	var got []string
	for {
		l := w.line(what)
		got = append(got, l)
		if strings.HasPrefix(l, tag+" ") {
			return got
		}
	}
}

// TestIDInsideLiteralIsNotACommand: an ID that appears inside literal payload
// is message data, not a command. Handling ID at the connection level rather
// than in the parser meant a body line could be answered as a command and
// removed from the stream, which cost the client the command that followed
// (yarilomail/yarilo#1375). Parsed here, the distinction is structural.
func TestIDInsideLiteralIsNotACommand(t *testing.T) {
	w := dialID(t)
	fmt.Fprint(w.conn, "a1 LOGIN u p\r\n")
	w.until("a1", "login")

	body := "From: a@b\r\nSubject: t\r\nX ID here\r\n\r\nhello\r\n"
	fmt.Fprintf(w.conn, "a2 APPEND INBOX {%d+}\r\n%s\r\n", len(body), body)
	for _, l := range w.until("a2", "append") {
		if strings.Contains(l, "ID completed") || strings.HasPrefix(l, "* ID ") {
			t.Fatalf("a line inside the literal was answered as an ID command: %q", l)
		}
	}
	fmt.Fprint(w.conn, "a3 NOOP\r\n")
	if got := w.line("noop"); !strings.HasPrefix(got, "a3 ") {
		t.Fatalf("the command after APPEND was not answered; got %q", got)
	}
}

// TestIDIsAnsweredAndItsLiteralsAreParsed: ID may carry its arguments as
// literals, which makes it span several lines. The parser handles that; a
// line-level interceptor had to be taught it separately.
func TestIDIsAnsweredAndItsLiteralsAreParsed(t *testing.T) {
	w := dialID(t)
	name := "probe"
	fmt.Fprintf(w.conn, "a1 ID (\"name\" {%d+}\r\n%s)\r\n", len(name), name)
	got := strings.Join(w.until("a1", "id"), "\n")
	if !strings.Contains(got, `"name" "test-server"`) {
		t.Errorf("server ID data missing: %q", got)
	}
	fmt.Fprint(w.conn, "a2 NOOP\r\n")
	if l := w.line("noop"); !strings.HasPrefix(l, "a2 ") {
		t.Fatalf("the command after a literal-carrying ID was not answered; got %q", l)
	}
}

// TestIDIsAdvertisedBeforeLogin: a client only sends ID if CAPABILITY says it
// may, and it sends it before authenticating. Advertising it only after login
// would leave the pre-auth identification RFC 2971 describes unreachable.
func TestIDIsAdvertisedBeforeLogin(t *testing.T) {
	w := dialID(t)
	fmt.Fprint(w.conn, "a1 CAPABILITY\r\n")
	got := strings.Join(w.until("a1", "capability"), "\n")
	if !strings.Contains(got, " ID") {
		t.Errorf("CAPABILITY does not advertise ID before login: %q", got)
	}
}
