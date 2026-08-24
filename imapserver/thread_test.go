package imapserver_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// threadSession records what the parser handed it, because half of what the
// dispatcher owes the session is arriving with the command intact.
type threadSession struct {
	imapserver.Session
	got *threadCall
}

type threadCall struct {
	kind     imapserver.NumKind
	alg      imap.ThreadAlgorithm
	criteria *imap.SearchCriteria
}

func (s *threadSession) Thread(kind imapserver.NumKind, alg imap.ThreadAlgorithm, criteria *imap.SearchCriteria) ([]imap.ThreadNode, error) {
	*s.got = threadCall{kind: kind, alg: alg, criteria: criteria}
	return []imap.ThreadNode{
		{Num: 2},
		{Num: 3, Children: []imap.ThreadNode{{Num: 6}}},
	}, nil
}

func startThreadServer(t *testing.T, withThread bool, algs ...imap.ThreadAlgorithm) (string, *threadCall) {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)

	caps := imap.CapSet{imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}}
	for _, alg := range algs {
		caps[imap.ThreadCap(alg)] = struct{}{}
	}
	got := &threadCall{}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			if !withThread {
				return mem.NewSession(), nil, nil
			}
			return &threadSession{Session: mem.NewSession(), got: got}, nil, nil
		},
		InsecureAuth: true,
		Caps:         caps,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go srv.Serve(ln) //nolint:errcheck
	return ln.Addr().String(), got
}

func dialThread(t *testing.T, addr string) *wire {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	w := &wire{t: t, conn: c, rd: bufio.NewReader(c)}
	w.line("greeting")
	fmt.Fprint(w.conn, "a1 LOGIN u p\r\n")
	w.until("a1", "login")
	fmt.Fprint(w.conn, "a2 SELECT INBOX\r\n")
	w.until("a2", "select")
	return w
}

// The command reaches the session with its three arguments parsed, and the
// reply is the tree the session returned. THREAD and UID THREAD differ only in
// the kind, which the session needs in order to number its answer.
func TestThreadIsParsedAndAnswered(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want imapserver.NumKind
	}{
		{"sequence numbers", "a3 THREAD REFERENCES UTF-8 ALL", imapserver.NumKindSeq},
		{"uids", "a3 UID THREAD REFERENCES UTF-8 ALL", imapserver.NumKindUID},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, got := startThreadServer(t, true, imap.ThreadReferences)
			w := dialThread(t, addr)
			fmt.Fprintf(w.conn, "%s\r\n", tc.cmd)
			lines := w.until("a3", "thread")

			if got.kind != tc.want {
				t.Errorf("session was told kind %v, want %v", got.kind, tc.want)
			}
			if got.alg != imap.ThreadReferences {
				t.Errorf("session was told algorithm %q, want REFERENCES", got.alg)
			}
			if got.criteria == nil {
				t.Error("session got no search criteria")
			}
			if reply := strings.Join(lines, "\n"); !strings.Contains(reply, "* THREAD (2)(3 6)") {
				t.Errorf("reply = %q, want the session's tree", reply)
			}
		})
	}
}

// The algorithm is checked against what the server announced, so a client can
// never be told one set of algorithms in CAPABILITY and served another.
func TestThreadRefusesAnAlgorithmItDidNotAnnounce(t *testing.T) {
	addr, got := startThreadServer(t, true, imap.ThreadReferences)
	w := dialThread(t, addr)
	fmt.Fprint(w.conn, "a3 THREAD ORDEREDSUBJECT UTF-8 ALL\r\n")
	lines := w.until("a3", "thread")

	if !strings.HasPrefix(lines[len(lines)-1], "a3 BAD") {
		t.Errorf("reply = %q, want BAD for an unannounced algorithm", lines)
	}
	if got.alg != "" {
		t.Errorf("the session was asked for %q anyway", got.alg)
	}
}

// RFC 5256 §4 has no form of the command without a charset, unlike SEARCH, so
// omitting it is a parse error rather than a silent US-ASCII assumption.
func TestThreadWithoutACharsetIsRejected(t *testing.T) {
	addr, got := startThreadServer(t, true, imap.ThreadReferences)
	w := dialThread(t, addr)
	fmt.Fprint(w.conn, "a3 THREAD REFERENCES ALL\r\n")
	lines := w.until("a3", "thread")

	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "a3 BAD") {
		t.Errorf("reply = %q, want BAD", last)
	}
	if got.alg != "" {
		t.Errorf("the session was asked to thread %q despite the malformed command", got.alg)
	}
}

// A session that does not implement threading answers BAD rather than
// panicking or reporting an empty set of threads -- an empty reply would say
// "no conversations here", which is a different statement.
func TestThreadWithoutSessionSupportIsRefused(t *testing.T) {
	addr, _ := startThreadServer(t, false)
	w := dialThread(t, addr)
	fmt.Fprint(w.conn, "a3 THREAD REFERENCES UTF-8 ALL\r\n")
	lines := w.until("a3", "thread")

	reply := strings.Join(lines, "\n")
	if strings.Contains(reply, "* THREAD") {
		t.Errorf("reply = %q, want no THREAD data at all", reply)
	}
	if !strings.HasPrefix(lines[len(lines)-1], "a3 BAD") {
		t.Errorf("reply = %q, want BAD", lines[len(lines)-1])
	}
}
