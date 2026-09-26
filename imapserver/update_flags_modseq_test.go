package imapserver

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// An unsolicited flag change carries the message's mod-sequence once the
// client enabled CONDSTORE (RFC 7162 3.2.4), and not before.
func TestUpdateWriterFlagsCarryModSeq(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled []imap.Cap
		modSeq  uint64
		want    string
	}{
		{"CONDSTORE enabled", []imap.Cap{imap.CapCondStore}, 42, "* 3 FETCH (UID 7 FLAGS (\\Seen) MODSEQ (42))\r\n"},
		{"not enabled", nil, 42, "* 3 FETCH (UID 7 FLAGS (\\Seen))\r\n"},
		{"enabled, no modseq", []imap.Cap{imap.CapCondStore}, 0, "* 3 FETCH (UID 7 FLAGS (\\Seen))\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer client.Close()
			c := newConn(server, New(&Options{}))
			for _, cp := range tc.enabled {
				c.enabled[cp] = struct{}{}
			}
			w := &UpdateWriter{conn: c}
			done := make(chan error, 1)
			go func() {
				done <- w.WriteMessageFlagsModSeq(3, 7, []imap.Flag{imap.FlagSeen}, tc.modSeq)
				server.Close()
			}()
			line, err := bufio.NewReader(client).ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if !strings.EqualFold(line, tc.want) {
				t.Errorf("wrote %q, want %q", line, tc.want)
			}
		})
	}
}
