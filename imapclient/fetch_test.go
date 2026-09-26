package imapclient_test

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestFetch(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	seqSet := imap.SeqSetNum(1)
	bodySection := &imap.FetchItemBodySection{}
	fetchOptions := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{bodySection},
	}
	messages, err := client.Fetch(seqSet, fetchOptions).Collect()
	if err != nil {
		t.Fatalf("failed to fetch first message: %v", err)
	} else if len(messages) != 1 {
		t.Fatalf("len(messages) = %v, want 1", len(messages))
	}

	msg := messages[0]
	if len(msg.BodySection) != 1 {
		t.Fatalf("len(msg.BodySection) = %v, want 1", len(msg.BodySection))
	}
	b := msg.FindBodySection(bodySection)
	if b == nil {
		t.Fatalf("FindBodySection() = nil")
	}
	body := strings.ReplaceAll(string(b), "\r\n", "\n")
	if body != simpleRawMessage {
		t.Errorf("body mismatch: got \n%v\n but want \n%v", body, simpleRawMessage)
	}
}

// A partial BINARY fetch is answered with its origin (RFC 3516 4.3), and the
// client reads it back and matches the response to what it asked.
func TestFetchBinaryPartialOrigin(t *testing.T) {
	client, server := newClientServerPair(t, imap.ConnStateSelected)
	defer client.Close()
	defer server.Close()

	whole, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		BinarySection: []*imap.FetchItemBinarySection{{Peek: true}},
	}).Collect()
	if err != nil || len(whole) != 1 {
		t.Fatalf("FETCH BINARY[] = %v, %v", whole, err)
	}
	full := whole[0].FindBinarySection(&imap.FetchItemBinarySection{})

	for _, tc := range []struct {
		name    string
		partial imap.SectionPartial
		want    []byte
	}{
		{"from the start", imap.SectionPartial{Offset: 0, Size: 5}, full[:5]},
		{"from an origin", imap.SectionPartial{Offset: 7, Size: 4}, full[7:11]},
		{"past the end", imap.SectionPartial{Offset: int64(len(full)) + 10, Size: 4}, []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			section := &imap.FetchItemBinarySection{Peek: true, Partial: &tc.partial}
			msgs, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
				BinarySection: []*imap.FetchItemBinarySection{section},
			}).Collect()
			if err != nil || len(msgs) != 1 {
				t.Fatalf("FETCH = %v, %v", msgs, err)
			}
			if len(msgs[0].BinarySection) != 1 || msgs[0].BinarySection[0].Section.Partial == nil ||
				msgs[0].BinarySection[0].Section.Partial.Offset != tc.partial.Offset {
				t.Fatalf("response section %+v, want origin %d", msgs[0].BinarySection, tc.partial.Offset)
			}
			got := msgs[0].FindBinarySection(section)
			if string(got) != string(tc.want) {
				t.Errorf("BINARY[]<%d.%d> = %q, want %q", tc.partial.Offset, tc.partial.Size, got, tc.want)
			}
		})
	}
}
