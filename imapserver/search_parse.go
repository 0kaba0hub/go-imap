package imapserver

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// ParseSearchCriteria reads one SEARCH key sequence from text, with the same
// reader the server uses on the wire. A server that keeps search criteria in
// configuration can then refuse a bad one where it is read, rather than when
// a client first runs it.
//
// The text is the key sequence alone: no tag, no SEARCH verb, no CRLF, and no
// RETURN or CHARSET prefix.
func ParseSearchCriteria(text string) (*imap.SearchCriteria, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("imapserver: the search criteria are empty")
	}
	// The decoder reads a command line, so it is handed one.
	dec := imapwire.NewDecoder(bufio.NewReader(strings.NewReader(text+"\r\n")), imapwire.ConnSideServer)

	var criteria imap.SearchCriteria
	var atom string
	maybeReadSearchKeyAtom(dec, &atom)
	for {
		var err error
		if atom != "" {
			err = readSearchKeyWithAtom(&criteria, dec, atom)
			atom = ""
		} else {
			err = readSearchKey(&criteria, dec)
		}
		if err != nil {
			return nil, fmt.Errorf("imapserver: in search-key: %w", err)
		}
		if !dec.SP() {
			break
		}
	}
	if !dec.ExpectCRLF() {
		return nil, fmt.Errorf("imapserver: trailing input after the search criteria: %w", dec.Err())
	}
	return &criteria, nil
}
