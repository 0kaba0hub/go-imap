package imapserver

import (
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleNotify parses the NOTIFY command (RFC 5465) and hands the resulting
// options to the session. NOTIFY NONE passes nil (disable); NOTIFY SET passes
// the parsed event groups.
func (c *Conn) handleNotify(dec *imapwire.Decoder) error {
	sess, ok := c.session.(SessionNotify)
	if !ok {
		dec.DiscardLine()
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "NOTIFY not supported"}
	}

	if !dec.ExpectSP() {
		return dec.Err()
	}
	var kind string
	if !dec.ExpectAtom(&kind) {
		return dec.Err()
	}

	var options *imap.NotifyOptions
	switch strings.ToUpper(kind) {
	case "NONE":
		// options stays nil — disable notifications.
	case "SET":
		opts := &imap.NotifyOptions{}
		for dec.SP() {
			if dec.Special('(') {
				item, err := parseNotifyEventGroup(dec)
				if err != nil {
					return err
				}
				if !dec.ExpectSpecial(')') {
					return dec.Err()
				}
				opts.Items = append(opts.Items, item)
				continue
			}
			var word string
			if !dec.ExpectAtom(&word) {
				return dec.Err()
			}
			if !strings.EqualFold(word, "STATUS") {
				return newClientBugError("NOTIFY: expected STATUS or an event group")
			}
			opts.Status = true
		}
		if len(opts.Items) == 0 {
			return newClientBugError("NOTIFY SET: at least one event group is required")
		}
		options = opts
	default:
		return newClientBugError("NOTIFY: expected NONE or SET")
	}

	if !dec.ExpectCRLF() {
		return dec.Err()
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}

	w := &UpdateWriter{conn: c, allowExpunge: true}
	return sess.Notify(w, options)
}

// parseNotifyEventGroup parses "filter SP events" — the body of an event group
// whose surrounding parentheses are consumed by the caller.
func parseNotifyEventGroup(dec *imapwire.Decoder) (imap.NotifyItem, error) {
	var item imap.NotifyItem

	var filter string
	if !dec.ExpectAtom(&filter) {
		return item, dec.Err()
	}
	switch strings.ToUpper(filter) {
	case "SELECTED", "SELECTED-DELAYED", "INBOXES", "PERSONAL", "SUBSCRIBED":
		item.MailboxSpec = imap.NotifyMailboxSpec(strings.ToUpper(filter))
	case "SUBTREE", "SUBTREE-ONE":
		item.Subtree = true
		if !dec.ExpectSP() {
			return item, dec.Err()
		}
		if err := parseNotifyMailboxes(dec, &item.Mailboxes); err != nil {
			return item, err
		}
	case "MAILBOXES":
		if !dec.ExpectSP() {
			return item, dec.Err()
		}
		if err := parseNotifyMailboxes(dec, &item.Mailboxes); err != nil {
			return item, err
		}
	default:
		return item, newClientBugError("NOTIFY: unknown mailbox filter " + filter)
	}

	if !dec.ExpectSP() {
		return item, dec.Err()
	}
	if err := parseNotifyEvents(dec, &item); err != nil {
		return item, err
	}
	return item, nil
}

// parseNotifyMailboxes parses one-or-more-mailbox: a single mailbox or a
// parenthesised list of mailboxes.
func parseNotifyMailboxes(dec *imapwire.Decoder, out *[]string) error {
	isList, err := dec.List(func() error {
		var mb string
		if !dec.ExpectMailbox(&mb) {
			return dec.Err()
		}
		*out = append(*out, mb)
		return nil
	})
	if err != nil {
		return err
	}
	if !isList {
		var mb string
		if !dec.ExpectMailbox(&mb) {
			return dec.Err()
		}
		*out = append(*out, mb)
	}
	return nil
}

// parseNotifyEvents parses events: NONE or a "(" event-list ")". An event may be
// followed by a parenthesised fetch-att spec (only MessageNew); it is consumed
// and, for now, ignored.
func parseNotifyEvents(dec *imapwire.Decoder, item *imap.NotifyItem) error {
	isList, err := dec.List(func() error {
		// A parenthesised group here is a fetch-att spec attached to the
		// preceding MessageNew event — consume and discard it.
		sub, serr := dec.List(func() error {
			if !dec.DiscardValue() {
				return dec.Err()
			}
			return nil
		})
		if serr != nil {
			return serr
		}
		if sub {
			return nil
		}
		var ev string
		if !dec.ExpectAtom(&ev) {
			return dec.Err()
		}
		item.Events = append(item.Events, imap.NotifyEvent(ev))
		return nil
	})
	if err != nil {
		return err
	}
	if !isList {
		var none string
		if !dec.ExpectAtom(&none) {
			return dec.Err()
		}
		if !strings.EqualFold(none, "NONE") {
			return newClientBugError("NOTIFY: expected event list or NONE")
		}
		// Empty Events = suppress the default events for this mailbox set.
	}
	return nil
}
