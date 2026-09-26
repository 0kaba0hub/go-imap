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

// A CONDSTORE enabling command enables CONDSTORE without ENABLE, so later
// unsolicited updates carry MODSEQ (RFC 7162 3.1); a plain SELECT does not.
func TestCondStoreEnablingCommands(t *testing.T) {
	for _, tc := range []struct {
		name, cmd string
		want      bool
	}{
		{"SELECT (CONDSTORE)", "SELECT INBOX (CONDSTORE)", true},
		{"STATUS HIGHESTMODSEQ", "STATUS INBOX (HIGHESTMODSEQ)", true},
		{"FETCH MODSEQ", "SELECT INBOX\r\nx FETCH 1:* (MODSEQ)", true},
		{"FETCH CHANGEDSINCE", "SELECT INBOX\r\nx FETCH 1:* (FLAGS) (CHANGEDSINCE 1)", true},
		{"FETCH CHANGEDSINCE 0", "SELECT INBOX\r\nx FETCH 1:* (FLAGS) (CHANGEDSINCE 0)", true},
		{"STORE UNCHANGEDSINCE", "SELECT INBOX\r\nx STORE 1 (UNCHANGEDSINCE 1) +FLAGS (\\Seen)", true},
		{"SEARCH MODSEQ", "SELECT INBOX\r\nx SEARCH MODSEQ 1", true},
		{"plain SELECT", "SELECT INBOX", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := imapmemserver.New()
			user := imapmemserver.NewUser("u", "p")
			user.Create("INBOX", nil)
			mem.AddUser(user)
			conns := make(chan *imapserver.Conn, 1)
			srv := imapserver.New(&imapserver.Options{
				NewSession: func(c *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
					conns <- c
					return mem.NewSession(), nil, nil
				},
				InsecureAuth: true,
				Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapCondStore: {}},
			})
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			go srv.Serve(ln) //nolint:errcheck
			nc, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer nc.Close()
			rd := bufio.NewReader(nc)
			conn := <-conns
			rd.ReadString('\n') //nolint:errcheck // greeting
			run := func(tag, line string) {
				fmt.Fprintf(nc, "%s %s\r\n", tag, line)
				for {
					l, err := rd.ReadString('\n')
					if err != nil {
						t.Fatal(err)
					}
					if strings.HasPrefix(l, tag+" ") {
						return
					}
				}
			}
			run("a", "LOGIN u p")
			lines := strings.Split(tc.cmd, "\r\nx ")
			for i, l := range lines {
				run(fmt.Sprintf("c%d", i), l)
			}
			if got := conn.EnabledCaps().Has(imap.CapCondStore); got != tc.want {
				t.Errorf("CONDSTORE enabled %v after %q, want %v", got, tc.cmd, tc.want)
			}
		})
	}
}
