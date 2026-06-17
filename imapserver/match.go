package imapserver

import (
	"bytes"
	"io"
	"strings"
	"time"

	imap "github.com/emersion/go-imap/v2"
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// MatchMessage reports whether a message matches criteria.
//
// Parameters:
//   - seqNum: 1-based sequence number (0 = unknown, SeqNum criteria then fail)
//   - uid: IMAP UID
//   - internalDate: INTERNALDATE for SINCE/BEFORE comparisons
//   - size: RFC822.SIZE in bytes for LARGER/SMALLER comparisons
//   - flags: current flag set
//   - rawMsg: full RFC 5322 message bytes (read only when criteria.Header,
//     criteria.Body, criteria.Text, criteria.SentSince, criteria.SentBefore,
//     or criteria.Not/Or recurse into those — may be nil if the caller knows
//     those fields are absent)
func MatchMessage(
	seqNum uint32,
	uid imap.UID,
	internalDate time.Time,
	size int64,
	flags []imap.Flag,
	rawMsg []byte,
	criteria *imap.SearchCriteria,
) bool {
	if criteria == nil {
		return true
	}

	for _, seqSet := range criteria.SeqNum {
		if seqNum == 0 || !seqSet.Contains(seqNum) {
			return false
		}
	}
	for _, uidSet := range criteria.UID {
		if !uidSet.Contains(uid) {
			return false
		}
	}

	if !matchDate(internalDate, criteria.Since, criteria.Before) {
		return false
	}

	flagMap := make(map[imap.Flag]struct{}, len(flags))
	for _, f := range flags {
		flagMap[canonicalFlag(f)] = struct{}{}
	}
	for _, f := range criteria.Flag {
		if _, ok := flagMap[canonicalFlag(f)]; !ok {
			return false
		}
	}
	for _, f := range criteria.NotFlag {
		if _, ok := flagMap[canonicalFlag(f)]; ok {
			return false
		}
	}

	if criteria.Larger != 0 && size <= criteria.Larger {
		return false
	}
	if criteria.Smaller != 0 && size >= criteria.Smaller {
		return false
	}

	// Content-based criteria require the raw message.
	needsContent := len(criteria.Header) > 0 ||
		len(criteria.Body) > 0 ||
		len(criteria.Text) > 0 ||
		!criteria.SentSince.IsZero() ||
		!criteria.SentBefore.IsZero()

	var entity *gomessage.Entity
	if needsContent && len(rawMsg) > 0 {
		e, err := gomessage.Read(bytes.NewReader(rawMsg))
		if err != nil && !gomessage.IsUnknownCharset(err) {
			return false
		}
		entity = e
	}

	if entity != nil {
		hdr := mail.Header{Header: entity.Header}

		for _, hc := range criteria.Header {
			if !matchHeaderFields(hdr.FieldsByKey(hc.Key), hc.Value) {
				return false
			}
		}

		if !criteria.SentSince.IsZero() || !criteria.SentBefore.IsZero() {
			t, err := hdr.Date()
			if err != nil {
				return false
			}
			if !matchDate(t, criteria.SentSince, criteria.SentBefore) {
				return false
			}
		}

		for _, text := range criteria.Text {
			if !matchEntity(entity, text, true) {
				return false
			}
		}
		for _, body := range criteria.Body {
			if !matchEntity(entity, body, false) {
				return false
			}
		}
	}

	for i := range criteria.Not {
		if MatchMessage(seqNum, uid, internalDate, size, flags, rawMsg, &criteria.Not[i]) {
			return false
		}
	}
	for i := range criteria.Or {
		a := MatchMessage(seqNum, uid, internalDate, size, flags, rawMsg, &criteria.Or[i][0])
		b := MatchMessage(seqNum, uid, internalDate, size, flags, rawMsg, &criteria.Or[i][1])
		if !a && !b {
			return false
		}
	}

	return true
}

func matchDate(t, since, before time.Time) bool {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if !since.IsZero() && t.Before(since) {
		return false
	}
	if !before.IsZero() && !t.Before(before) {
		return false
	}
	return true
}

func matchHeaderFields(fields gomessage.HeaderFields, pattern string) bool {
	if pattern == "" {
		return fields.Len() > 0
	}
	pattern = strings.ToLower(pattern)
	for fields.Next() {
		v, _ := fields.Text()
		if strings.Contains(strings.ToLower(v), pattern) {
			return true
		}
	}
	return false
}

func matchEntity(e *gomessage.Entity, pattern string, includeHeader bool) bool {
	if pattern == "" {
		return true
	}
	if includeHeader && matchHeaderFields(e.Header.Fields(), pattern) {
		return true
	}
	if mr := e.MultipartReader(); mr != nil {
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			} else if err != nil {
				return false
			}
			if matchEntity(part, pattern, includeHeader) {
				return true
			}
		}
		return false
	}
	t, _, err := e.Header.ContentType()
	if err != nil {
		return false
	}
	if !strings.HasPrefix(t, "text/") && !strings.HasPrefix(t, "message/") {
		return false
	}
	buf, err := io.ReadAll(e.Body)
	if err != nil {
		return false
	}
	return bytes.Contains(bytes.ToLower(buf), bytes.ToLower([]byte(pattern)))
}

func canonicalFlag(f imap.Flag) imap.Flag {
	return imap.Flag(strings.ToLower(string(f)))
}
