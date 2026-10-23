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

type sortThreadSession struct {
	imapserver.Session
}

func (s *sortThreadSession) Sort(imapserver.NumKind, []imap.SortCriterion, *imap.SearchCriteria) ([]uint32, error) {
	return nil, nil
}

func (s *sortThreadSession) Thread(imapserver.NumKind, imap.ThreadAlgorithm, *imap.SearchCriteria) ([]imap.ThreadNode, error) {
	return nil, nil
}

func dialSortThread(t *testing.T) *wire {
	t.Helper()
	mem := imapmemserver.New()
	mem.AddUser(imapmemserver.NewUser("u", "p"))
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &sortThreadSession{Session: mem.NewSession()}, nil, nil
		},
		InsecureAuth: true,
		Caps: imap.CapSet{
			imap.CapIMAP4rev1:                         {},
			imap.CapSort:                              {},
			imap.ThreadCap(imap.ThreadReferences):     {},
			imap.ThreadCap(imap.ThreadOrderedSubject): {},
		},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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

func capabilities(w *wire, tag string) map[string]bool {
	fmt.Fprintf(w.conn, "%s CAPABILITY\r\n", tag)
	got := map[string]bool{}
	for _, l := range w.until(tag, "capability") {
		if rest, ok := strings.CutPrefix(l, "* CAPABILITY "); ok {
			for _, c := range strings.Fields(rest) {
				got[c] = true
			}
		}
	}
	return got
}

// A client uses SORT and THREAD only when CAPABILITY names them, so a server
// that answers both commands but leaves them out of the list is, to every
// client, a server without them. They are selected-state commands: offered
// after login, not before.
func TestCapabilityNamesSortAndThreadAfterLogin(t *testing.T) {
	want := []string{"SORT", "THREAD=REFERENCES", "THREAD=ORDEREDSUBJECT"}
	w := dialSortThread(t)

	before := capabilities(w, "a1")
	for _, c := range want {
		if before[c] {
			t.Errorf("%s is offered before login", c)
		}
	}

	fmt.Fprint(w.conn, "a2 LOGIN u p\r\n")
	w.until("a2", "login")
	after := capabilities(w, "a3")
	for _, c := range want {
		if !after[c] {
			t.Errorf("%s is missing from CAPABILITY after login", c)
		}
	}
}
