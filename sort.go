package imap

// SortKey is a SORT criterion key (RFC 5256 §3).
type SortKey string

const (
	SortKeyArrival SortKey = "ARRIVAL"
	SortKeyCc      SortKey = "CC"
	SortKeyDate    SortKey = "DATE"
	SortKeyFrom    SortKey = "FROM"
	SortKeySize    SortKey = "SIZE"
	SortKeySubject SortKey = "SUBJECT"
	SortKeyTo      SortKey = "TO"
)

// SortCriterion is one key of a SORT command, optionally reversed.
//
// REVERSE applies to its own key alone: it does not touch the implicit
// sequence-number tie-break, so REVERSE SUBJECT is not the reverse of a
// SUBJECT sort (RFC 5256 §3).
type SortCriterion struct {
	Key     SortKey
	Reverse bool
}
