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

// byeOnDelete closes the connection once DELETE is answered.
type byeOnDelete struct {
	*imapmemserver.UserSession
	conn *imapserver.Conn
}

func (s *byeOnDelete) Delete(name string) error {
	if err := s.UserSession.Delete(name); err != nil {
		return err
	}
	s.conn.AfterResponse(func() { s.conn.Bye("Selected mailbox was deleted, have to disconnect") })
	return nil
}

// A hook set by a handler runs after the tagged response is written: the
// client reads its OK before the BYE and the close.
func TestAfterResponseRunsAfterTheTaggedResponse(t *testing.T) {
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("u", "p")
	user.Create("INBOX", nil)
	user.Create("Gone", nil)
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(c *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &byeOnDelete{UserSession: imapmemserver.NewUserSession(user), conn: c}, nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
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
	fmt.Fprintf(nc, "a LOGIN u p\r\nb DELETE Gone\r\n")

	var lines []string
	for {
		l, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		lines = append(lines, strings.TrimRight(l, "\r\n"))
	}
	want := []string{"b OK DELETE completed", "* BYE Selected mailbox was deleted, have to disconnect"}
	if len(lines) < 2 || lines[len(lines)-2] != want[0] || lines[len(lines)-1] != want[1] {
		t.Errorf("the connection ended with %q, want %q", lines, want)
	}
}
