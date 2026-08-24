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

type sortSession struct {
	imapserver.Session
	got    *sortCall
	result []uint32
}

type sortCall struct {
	kind     imapserver.NumKind
	criteria []imap.SortCriterion
	search   *imap.SearchCriteria
}

func (s *sortSession) Sort(kind imapserver.NumKind, criteria []imap.SortCriterion, search *imap.SearchCriteria) ([]uint32, error) {
	*s.got = sortCall{kind: kind, criteria: criteria, search: search}
	return s.result, nil
}

func startSortServer(t *testing.T, withSort, announce bool) (string, *sortCall) {
	return startSortServerResult(t, withSort, announce, []uint32{2, 84, 882})
}

func startSortServerResult(t *testing.T, withSort, announce bool, result []uint32) (string, *sortCall) {
	t.Helper()
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)

	caps := imap.CapSet{imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}}
	if announce {
		caps[imap.CapSort] = struct{}{}
	}
	got := &sortCall{}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			if !withSort {
				return mem.NewSession(), nil, nil
			}
			return &sortSession{Session: mem.NewSession(), got: got, result: result}, nil, nil
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

func dialSort(t *testing.T, addr string) *wire {
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

// The criteria reach the session in order and with REVERSE attached to its own
// key -- the example is RFC 5256 §3's own, where only DATE is reversed.
func TestSortCriteriaArriveInOrder(t *testing.T) {
	addr, got := startSortServer(t, true, true)
	w := dialSort(t, addr)
	fmt.Fprint(w.conn, "a3 SORT (SUBJECT REVERSE DATE) UTF-8 ALL\r\n")
	lines := w.until("a3", "sort")

	want := []imap.SortCriterion{
		{Key: imap.SortKeySubject},
		{Key: imap.SortKeyDate, Reverse: true},
	}
	if len(got.criteria) != len(want) {
		t.Fatalf("session got %d criteria (%v), want %d", len(got.criteria), got.criteria, len(want))
	}
	for i := range want {
		if got.criteria[i] != want[i] {
			t.Errorf("criterion %d = %+v, want %+v", i, got.criteria[i], want[i])
		}
	}
	if reply := strings.Join(lines, "\n"); !strings.Contains(reply, "* SORT 2 84 882") {
		t.Errorf("reply = %q, want the session's order", reply)
	}
}

func TestUIDSortTellsTheSessionSo(t *testing.T) {
	addr, got := startSortServer(t, true, true)
	w := dialSort(t, addr)
	fmt.Fprint(w.conn, "a3 UID SORT (ARRIVAL) UTF-8 ALL\r\n")
	w.until("a3", "sort")

	if got.kind != imapserver.NumKindUID {
		t.Errorf("session was told kind %v, want UID", got.kind)
	}
}

// An empty result is the keyword with nothing after it (RFC 5256 §3's own
// example), not silence and not an error.
func TestSortWithNoMatchesWritesTheBareKeyword(t *testing.T) {
	addr, _ := startSortServerResult(t, true, true, nil)
	w := dialSort(t, addr)
	fmt.Fprint(w.conn, "a3 SORT (SIZE) UTF-8 ALL\r\n")

	var got string
	for _, l := range w.until("a3", "sort") {
		if strings.HasPrefix(l, "* SORT") {
			got = l
		}
	}
	if got != "* SORT" {
		t.Errorf("empty result wrote %q, want the bare keyword -- silence would leave the client with no answer at all", got)
	}
}

func TestSortRejectsWhatItDoesNotImplement(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		// RFC 5957, the SORT=DISPLAY capability, which is not announced.
		{"a display key", "a3 SORT (DISPLAYFROM) UTF-8 ALL"},
		{"an invented key", "a3 SORT (COLOUR) UTF-8 ALL"},
		// The grammar has no form without a charset.
		{"no charset", "a3 SORT (SUBJECT) ALL"},
		// sort-criteria requires at least one criterion.
		{"no criteria", "a3 SORT () UTF-8 ALL"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, got := startSortServer(t, true, true)
			w := dialSort(t, addr)
			fmt.Fprintf(w.conn, "%s\r\n", tc.cmd)
			lines := w.until("a3", "sort")

			last := lines[len(lines)-1]
			if !strings.HasPrefix(last, "a3 BAD") && !strings.HasPrefix(last, "a3 NO") {
				t.Errorf("reply = %q, want a refusal", last)
			}
			if got.criteria != nil {
				t.Errorf("the session was asked to sort %v anyway", got.criteria)
			}
		})
	}
}

// A session without sorting answers BAD rather than an empty "* SORT", which
// would state that no message matched.
func TestSortWithoutSessionSupportIsRefused(t *testing.T) {
	addr, _ := startSortServer(t, false, false)
	w := dialSort(t, addr)
	fmt.Fprint(w.conn, "a3 SORT (SUBJECT) UTF-8 ALL\r\n")
	lines := w.until("a3", "sort")

	if reply := strings.Join(lines, "\n"); strings.Contains(reply, "* SORT") {
		t.Errorf("reply = %q, want no SORT data", reply)
	}
	if !strings.HasPrefix(lines[len(lines)-1], "a3 BAD") {
		t.Errorf("reply = %q, want BAD", lines[len(lines)-1])
	}
}
