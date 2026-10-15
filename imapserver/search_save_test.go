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

// RETURN (SAVE) alone answers no ESEARCH but still saves $; SAVE with
// another option answers only that option (RFC 5182 §2.1).
func TestSearchReturnSave(t *testing.T) {
	for _, tc := range []struct {
		name, cmd, want string
	}{
		{"SAVE alone", "UID SEARCH RETURN (SAVE) ALL", ""},
		{"SAVE then $", "UID SEARCH RETURN (SAVE) UID 2\r\nx UID SEARCH RETURN (ALL) UID $", `* ESEARCH (TAG "c1") UID ALL 2`},
		{"SAVE with COUNT", "UID SEARCH RETURN (SAVE COUNT) ALL", `* ESEARCH (TAG "c0") UID COUNT 3`},
		{"no option", "UID SEARCH RETURN () ALL", `* ESEARCH (TAG "c0") UID ALL 1:3`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := imapmemserver.New()
			user := imapmemserver.NewUser("u", "p")
			user.Create("INBOX", nil)
			mem.AddUser(user)
			srv := imapserver.New(&imapserver.Options{
				NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
					return mem.NewSession(), nil, nil
				},
				InsecureAuth: true,
				Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapESearch: {}, imap.CapSearchRes: {}},
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
			rd.ReadString('\n') //nolint:errcheck // greeting
			wait := func(tag string) []string {
				var untagged []string
				for {
					l, err := rd.ReadString('\n')
					if err != nil {
						t.Fatal(err)
					}
					l = strings.TrimRight(l, "\r\n")
					if strings.HasPrefix(l, tag+" ") {
						if !strings.HasPrefix(l, tag+" OK") {
							t.Fatalf("%s", l)
						}
						return untagged
					}
					untagged = append(untagged, l)
				}
			}
			run := func(tag, line string) []string {
				fmt.Fprintf(nc, "%s %s\r\n", tag, line)
				return wait(tag)
			}
			run("a", "LOGIN u p")
			for i := 1; i <= 3; i++ {
				msg := fmt.Sprintf("Subject: %d\r\n\r\nbody\r\n", i)
				fmt.Fprintf(nc, "p%d APPEND INBOX {%d}\r\n", i, len(msg))
				rd.ReadString('\n') //nolint:errcheck // continuation
				fmt.Fprintf(nc, "%s\r\n", msg)
				wait(fmt.Sprintf("p%d", i))
			}
			run("s", "SELECT INBOX")
			var esearch []string
			for i, l := range strings.Split(tc.cmd, "\r\nx ") {
				esearch = nil
				for _, u := range run(fmt.Sprintf("c%d", i), l) {
					if strings.HasPrefix(u, "* ESEARCH") {
						esearch = append(esearch, u)
					}
				}
			}
			if got := strings.Join(esearch, "\n"); got != tc.want {
				t.Errorf("%q answered %q, want %q", tc.cmd, got, tc.want)
			}
		})
	}
}
