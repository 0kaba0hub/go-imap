package imapserver

import (
	"bufio"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

func TestParseNotifyEventGroup(t *testing.T) {
	cases := []struct {
		in      string
		spec    imap.NotifyMailboxSpec
		mboxes  []string
		subtree bool
		events  []imap.NotifyEvent
	}{
		{
			in:     "(SELECTED (MessageNew MessageExpunge FlagChange))",
			spec:   imap.NotifyMailboxSpecSelected,
			events: []imap.NotifyEvent{"MessageNew", "MessageExpunge", "FlagChange"},
		},
		{
			in:     "(personal (MessageNew))",
			spec:   imap.NotifyMailboxSpecPersonal,
			events: []imap.NotifyEvent{"MessageNew"},
		},
		{
			in:     "(mailboxes (INBOX Work) (MessageNew MessageExpunge))",
			mboxes: []string{"INBOX", "Work"},
			events: []imap.NotifyEvent{"MessageNew", "MessageExpunge"},
		},
		{
			in:      "(subtree Archive (MessageExpunge))",
			mboxes:  []string{"Archive"},
			subtree: true,
			events:  []imap.NotifyEvent{"MessageExpunge"},
		},
		{
			in:   "(SELECTED NONE)",
			spec: imap.NotifyMailboxSpecSelected,
		},
		{
			// The MessageNew fetch-att spec is consumed and ignored.
			in:     "(SELECTED (MessageNew (UID FLAGS) FlagChange))",
			spec:   imap.NotifyMailboxSpecSelected,
			events: []imap.NotifyEvent{"MessageNew", "FlagChange"},
		},
	}

	for _, tc := range cases {
		dec := imapwire.NewDecoder(bufio.NewReader(strings.NewReader(tc.in)), 0)
		if !dec.Special('(') {
			t.Fatalf("%q: expected opening paren", tc.in)
		}
		item, err := parseNotifyEventGroup(dec)
		if err != nil {
			t.Fatalf("%q: parse: %v", tc.in, err)
		}
		if !dec.ExpectSpecial(')') {
			t.Fatalf("%q: expected closing paren: %v", tc.in, dec.Err())
		}
		if item.MailboxSpec != tc.spec {
			t.Errorf("%q: spec = %q, want %q", tc.in, item.MailboxSpec, tc.spec)
		}
		if !reflect.DeepEqual(item.Mailboxes, tc.mboxes) {
			t.Errorf("%q: mailboxes = %v, want %v", tc.in, item.Mailboxes, tc.mboxes)
		}
		if item.Subtree != tc.subtree {
			t.Errorf("%q: subtree = %v, want %v", tc.in, item.Subtree, tc.subtree)
		}
		if !reflect.DeepEqual(item.Events, tc.events) {
			t.Errorf("%q: events = %v, want %v", tc.in, item.Events, tc.events)
		}
	}
}
