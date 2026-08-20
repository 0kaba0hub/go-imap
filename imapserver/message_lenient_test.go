package imapserver

import (
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// A header line with no colon: the strict parser refuses the whole header, and
// every section used to come back empty while RFC822.SIZE reported the real
// size -- the client is told a message is there and cannot read it
// (yarilomail/yarilo#1377). Mail like this arrives; the reference serves it.
const brokenMsg = "From: a@b\r\n" +
	"Subject: t\r\n" +
	"X ID here\r\n" +
	"X-Note: kept\r\n" +
	"\r\n" +
	"hello\r\n"

func extract(t *testing.T, item *imap.FetchItemBodySection) string {
	t.Helper()
	return string(ExtractBodySection(strings.NewReader(brokenMsg), item))
}

func TestExtractBodySection_MalformedHeaderIsStillServed(t *testing.T) {
	t.Run("whole message", func(t *testing.T) {
		if got := extract(t, &imap.FetchItemBodySection{}); got != brokenMsg {
			t.Errorf("BODY[] = %q, want the message as stored", got)
		}
	})
	t.Run("header", func(t *testing.T) {
		want := "From: a@b\r\nSubject: t\r\nX ID here\r\nX-Note: kept\r\n\r\n"
		if got := extract(t, &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader}); got != want {
			t.Errorf("BODY[HEADER] = %q, want %q", got, want)
		}
	})
	t.Run("text", func(t *testing.T) {
		if got := extract(t, &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText}); got != "hello\r\n" {
			t.Errorf("BODY[TEXT] = %q, want the body after the empty line", got)
		}
	})
	t.Run("named fields", func(t *testing.T) {
		got := extract(t, &imap.FetchItemBodySection{
			Specifier:    imap.PartSpecifierHeader,
			HeaderFields: []string{"subject", "x-note"},
		})
		want := "Subject: t\r\nX-Note: kept\r\n\r\n"
		if got != want {
			t.Errorf("BODY[HEADER.FIELDS (SUBJECT X-NOTE)] = %q, want %q", got, want)
		}
	})
	t.Run("fields not", func(t *testing.T) {
		got := extract(t, &imap.FetchItemBodySection{
			Specifier:       imap.PartSpecifierHeader,
			HeaderFieldsNot: []string{"from"},
		})
		if strings.Contains(got, "From:") {
			t.Errorf("BODY[HEADER.FIELDS.NOT (FROM)] kept the excluded field: %q", got)
		}
		if !strings.Contains(got, "Subject: t") {
			t.Errorf("BODY[HEADER.FIELDS.NOT (FROM)] dropped a field it should keep: %q", got)
		}
	})
	t.Run("a named part is refused, not guessed", func(t *testing.T) {
		if got := ExtractBodySection(strings.NewReader(brokenMsg), &imap.FetchItemBodySection{Part: []int{1}}); got != nil {
			t.Errorf("BODY[1] = %q, want nil -- walking parts needs a parse", got)
		}
	})
}

// A well-formed message must keep going through the strict path unchanged.
func TestExtractBodySection_WellFormedIsUnchanged(t *testing.T) {
	const msg = "From: a@b\r\nSubject: t\r\n\r\nhello\r\n"
	got := string(ExtractBodySection(strings.NewReader(msg), &imap.FetchItemBodySection{}))
	if got != msg {
		t.Errorf("BODY[] = %q, want %q", got, msg)
	}
	body := string(ExtractBodySection(strings.NewReader(msg), &imap.FetchItemBodySection{Specifier: imap.PartSpecifierText}))
	if body != "hello\r\n" {
		t.Errorf("BODY[TEXT] = %q", body)
	}
}

func TestExtractBinarySection_MalformedHeaderServesTheMessage(t *testing.T) {
	got := string(ExtractBinarySection(strings.NewReader(brokenMsg), &imap.FetchItemBinarySection{}))
	if got != brokenMsg {
		t.Errorf("BINARY[] = %q, want the message as stored", got)
	}
}
