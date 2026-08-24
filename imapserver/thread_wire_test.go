package imapserver

import (
	"bufio"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

func node(num uint32, children ...imap.ThreadNode) imap.ThreadNode {
	return imap.ThreadNode{Num: num, Children: children}
}

// The shape of a THREAD reply is the whole answer: the same message numbers in
// different parentheses describe different conversations. So the assertion is
// on the bytes, and the inputs are the trees RFC 5256 §4 prints for itself --
// the specification supplies examples chosen to distinguish the cases, which
// is exactly what a writer test needs and what invented trees would not be.
func TestWriteThreadMatchesTheExamplesInRFC5256(t *testing.T) {
	tests := []struct {
		name  string
		roots []imap.ThreadNode
		want  string
	}{{
		// "* THREAD (2)(3 6 (4 23)(44 7 96))" -- the worked example of §4.
		// Message 3 is answered by 6; 6 has two answers, and each of those
		// continues on its own.
		name: "the example reply",
		roots: []imap.ThreadNode{
			node(2),
			node(3, node(6, node(4, node(23)), node(44, node(7, node(96))))),
		},
		want: "* THREAD (2)(3 6 (4 23)(44 7 96))\r\n",
	}, {
		// "* THREAD ((3)(5))" -- §4's own illustration of a dummy: 3 and 5 are
		// siblings whose parent is not in the mailbox. Flattening this to
		// "(3 5)" would claim 5 is a reply to 3.
		name: "siblings under a missing parent",
		roots: []imap.ThreadNode{
			node(0, node(3), node(5)),
		},
		want: "* THREAD ((3)(5))\r\n",
	}, {
		// A conversation with no branches stays flat however long it runs:
		// depth follows branching, not length.
		name: "a linear conversation",
		roots: []imap.ThreadNode{
			node(1, node(2, node(3, node(4)))),
		},
		want: "* THREAD (1 2 3 4)\r\n",
	}, {
		name:  "no messages match",
		roots: nil,
		want:  "* THREAD\r\n",
	}, {
		// Each root is its own list, with a single space between them and the
		// keyword, and none between the lists.
		name:  "unrelated messages",
		roots: []imap.ThreadNode{node(1), node(2), node(3)},
		want:  "* THREAD (1)(2)(3)\r\n",
	}, {
		// A dummy carrying a single child collapses to that child: there is no
		// branching for its parentheses to record.
		name:  "a dummy with one child",
		roots: []imap.ThreadNode{node(0, node(9, node(10)))},
		want:  "* THREAD (9 10)\r\n",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			bw := bufio.NewWriter(&sb)
			enc := imapwire.NewEncoder(bw, imapwire.ConnSideServer)
			writeThreadRoots(enc, tc.roots)
			if err := enc.CRLF(); err != nil {
				t.Fatal(err)
			}
			bw.Flush() //nolint:errcheck
			if got := sb.String(); got != tc.want {
				t.Errorf("wrote   %q\nwant    %q", got, tc.want)
			}
		})
	}
}
