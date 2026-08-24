package imapserver

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleSort implements SORT and UID SORT (RFC 5256 §3).
//
//	sort = ["UID" SP] "SORT" SP sort-criteria SP search-charset SP search-program
//
// As with THREAD, the charset is mandatory -- the specification says so in as
// many words ("unlike SEARCH") -- so there is no form of this command that
// silently assumes US-ASCII.
func (c *Conn) handleSort(dec *imapwire.Decoder, numKind NumKind) error {
	session, ok := c.session.(SessionSort)
	if !ok {
		return newClientBugError("SORT is not supported")
	}

	if !dec.ExpectSP() {
		return dec.Err()
	}
	criteria, err := readSortCriteria(dec)
	if err != nil {
		return err
	}

	var charset string
	if !dec.ExpectSP() || !dec.ExpectAString(&charset) || !dec.ExpectSP() {
		return dec.Err()
	}
	switch strings.ToUpper(charset) {
	case "US-ASCII", "UTF-8":
	default:
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeBadCharset,
			Text: "Only US-ASCII and UTF-8 are supported SORT charsets",
		}
	}

	var search imap.SearchCriteria
	for {
		if err := readSearchKey(&search, dec); err != nil {
			return fmt.Errorf("in search-key: %w", err)
		}
		if !dec.SP() {
			break
		}
	}
	if !dec.ExpectCRLF() {
		return dec.Err()
	}

	if err := c.checkState(imap.ConnStateSelected); err != nil {
		return err
	}

	nums, err := session.Sort(numKind, criteria, &search)
	if err != nil {
		return err
	}
	return c.writeSort(nums)
}

// readSortCriteria reads the parenthesised sort-criteria list. An empty list
// is refused: sorting by nothing is not a request this command can answer, and
// the grammar requires at least one criterion.
func readSortCriteria(dec *imapwire.Decoder) ([]imap.SortCriterion, error) {
	var criteria []imap.SortCriterion
	err := dec.ExpectList(func() error {
		var atom string
		if !dec.ExpectAtom(&atom) {
			return dec.Err()
		}
		var criterion imap.SortCriterion
		if strings.EqualFold(atom, "REVERSE") {
			// REVERSE is a prefix of the key that follows, not a key.
			criterion.Reverse = true
			if !dec.ExpectSP() || !dec.ExpectAtom(&atom) {
				return dec.Err()
			}
		}
		criterion.Key = imap.SortKey(strings.ToUpper(atom))
		switch criterion.Key {
		case imap.SortKeyArrival, imap.SortKeyCc, imap.SortKeyDate,
			imap.SortKeyFrom, imap.SortKeySize, imap.SortKeySubject, imap.SortKeyTo:
		default:
			// DISPLAYFROM and DISPLAYTO land here: they are RFC 5957 and
			// belong to the SORT=DISPLAY capability, which this server does
			// not announce.
			return &imap.Error{
				Type: imap.StatusResponseTypeBad,
				Text: fmt.Sprintf("Unsupported sort key %q", atom),
			}
		}
		criteria = append(criteria, criterion)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("in sort-criteria: %w", err)
	}
	if len(criteria) == 0 {
		return nil, newClientBugError("SORT requires at least one sort criterion")
	}
	return criteria, nil
}

func (c *Conn) writeSort(nums []uint32) error {
	enc := newResponseEncoder(c)
	defer enc.end()

	enc.Atom("*").SP().Atom("SORT")
	for _, num := range nums {
		enc.SP().Number(num)
	}
	return enc.CRLF()
}
