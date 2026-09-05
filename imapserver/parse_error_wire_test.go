package imapserver_test

import (
	"fmt"
	"strings"
	"testing"
)

// A command the grammar refuses is answered BAD, not NO [SERVERBUG].
//
// The connection loop answers BAD for a DecoderExpectError and NO [SERVERBUG]
// for anything else, so a parse failure that arrived as a plain error told the
// client the server was broken and sent the operator hunting a crash that never
// happened (yarilomail/yarilo#1689).
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

// An error from the handler is still NO [SERVERBUG]: this is a classification of
// parse failures, not "answer BAD to everything".
func TestAHandlerErrorIsStillServerBug(t *testing.T) {
	w := dialID(t)
	fmt.Fprint(w.conn, "a1 LOGIN u p\r\n")
	w.until("a1", "login")
	// SELECT of a mailbox that does not exist: well-formed, and the handler
	// refuses it. The answer must not become BAD.
	fmt.Fprint(w.conn, "a2 SELECT nosuchbox\r\n")
	last := ""
	for _, l := range w.until("a2", "select") {
		last = l
	}
	if strings.HasPrefix(last, "a2 BAD") {
		t.Errorf("a well-formed command the handler refused was answered %q", last)
	}
}
