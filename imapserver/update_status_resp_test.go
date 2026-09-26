package imapserver

import (
	"bufio"
	"net"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// A failure met while polling goes out untagged, so the command it follows
// still gets its own tagged answer.
func TestUpdateWriterStatusRespIsUntagged(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	w := &UpdateWriter{conn: newConn(server, New(&Options{}))}
	done := make(chan error, 1)
	go func() {
		done <- w.WriteStatusResp(&imap.StatusResponse{
			Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeServerBug, Text: "Internal error",
		})
		server.Close()
	}()
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if want := "* NO [SERVERBUG] Internal error\r\n"; line != want {
		t.Errorf("wrote %q, want %q", line, want)
	}
}
