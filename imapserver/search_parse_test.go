package imapserver_test

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

func TestParseSearchCriteria(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		check func(*testing.T, *imap.SearchCriteria)
	}{
		{"a flag key", "UNSEEN", func(t *testing.T, c *imap.SearchCriteria) {
			if len(c.NotFlag) != 1 || c.NotFlag[0] != imap.FlagSeen {
				t.Errorf("UNSEEN read as %+v", c)
			}
		}},
		{"two keys are ANDed", "UNSEEN SMALLER 4096", func(t *testing.T, c *imap.SearchCriteria) {
			if len(c.NotFlag) != 1 || c.Smaller != 4096 {
				t.Errorf("read as %+v", c)
			}
		}},
		{"a header key", `FROM "alice@example.com"`, func(t *testing.T, c *imap.SearchCriteria) {
			if len(c.Header) != 1 || c.Header[0].Key != "From" || c.Header[0].Value != "alice@example.com" {
				t.Errorf("read as %+v", c.Header)
			}
		}},
		{"a date key", "SINCE 1-Feb-2026", func(t *testing.T, c *imap.SearchCriteria) {
			want := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
			if !c.Since.Equal(want) {
				t.Errorf("SINCE read as %v, want %v", c.Since, want)
			}
		}},
		{"a parenthesised OR", `OR (UNSEEN) (FLAGGED)`, func(t *testing.T, c *imap.SearchCriteria) {
			if len(c.Or) != 1 {
				t.Errorf("OR read as %+v", c.Or)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := imapserver.ParseSearchCriteria(tc.text)
			if err != nil {
				t.Fatalf("%q: %v", tc.text, err)
			}
			tc.check(t, got)
		})
	}
}

func TestParseSearchCriteriaRefusals(t *testing.T) {
	for _, text := range []string{
		"",
		"   ",
		"NOTAKEY",
		"SMALLER",
		"SMALLER notanumber",
		"UNSEEN trailing garbage )",
	} {
		if got, err := imapserver.ParseSearchCriteria(text); err == nil {
			t.Errorf("%q was accepted as %+v", text, got)
		}
	}
}
