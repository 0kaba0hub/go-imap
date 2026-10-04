package imapserver_test

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// Parameters keep the message's order; RFC 2231 continuations come after the
// plain ones, merged. A sorted list was the writer's order, not the message's.
func TestBodyStructureParamsKeepMessageOrder(t *testing.T) {
	mem := imapmemserver.New()
	u := imapmemserver.NewUser("u", "p")
	if err := u.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(u)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: struct{}{}, imap.CapLiteralPlus: struct{}{}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck
	go srv.Serve(ln)                 //nolint:errcheck

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close() //nolint:errcheck
	rd := bufio.NewReader(c)
	read := func() string {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		line, rerr := rd.ReadString('\n')
		if rerr != nil {
			t.Fatalf("read: %v", rerr)
		}
		return strings.TrimRight(line, "\r\n")
	}
	run := func(tag, cmd string) (out []string) {
		if _, werr := c.Write([]byte(tag + " " + cmd + "\r\n")); werr != nil {
			t.Fatal(werr)
		}
		for {
			line := read()
			if strings.HasPrefix(line, tag+" ") {
				return out
			}
			out = append(out, line)
		}
	}
	read() // greeting
	msg := "Subject: order\r\n" +
		"Content-Type: text/plain; format=flowed; charset=utf-8\r\n" +
		"Content-Disposition: attachment; size=3; filename*1=\"b.txt\"; filename*0=\"a\"\r\n" +
		"\r\nabc\r\n"
	run("a", "login u p")
	run("b", "append INBOX {"+strconv.Itoa(len(msg))+"+}\r\n"+msg)
	run("c", "select INBOX")
	got := strings.Join(run("d", "fetch 1 (bodystructure)"), "\n")
	for _, want := range []string{
		`("format" "flowed" "charset" "utf-8")`,
		`("attachment" ("size" "3" "filename" "ab.txt"))`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}
