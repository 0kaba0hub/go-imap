package imapserver

import (
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// defaultAppendLimit is the default maximum size of an APPEND payload.
const defaultAppendLimit = 100 * 1024 * 1024 // 100MiB

func (c *Conn) handleAppend(tag string, dec *imapwire.Decoder) error {
	var (
		mailbox string
		options imap.AppendOptions
	)
	if !dec.ExpectSP() || !dec.ExpectMailbox(&mailbox) || !dec.ExpectSP() {
		return dec.Err()
	}

	hasFlagList, err := dec.List(func() error {
		flag, err := internal.ExpectFlag(dec)
		if err != nil {
			return err
		}
		options.Flags = append(options.Flags, flag)
		return nil
	})
	if err != nil {
		return err
	}
	if hasFlagList && !dec.ExpectSP() {
		return dec.Err()
	}

	t, err := internal.DecodeDateTime(dec)
	if err != nil {
		return err
	}
	if !t.IsZero() && !dec.ExpectSP() {
		return dec.Err()
	}
	options.Time = t

	var dataExt string
	if !dec.Special('~') && dec.Atom(&dataExt) { // ignore literal8 prefix if any for BINARY
		switch strings.ToUpper(dataExt) {
		case "UTF8":
			// '~' is the literal8 prefix
			if !dec.ExpectSP() || !dec.ExpectSpecial('(') || !dec.ExpectSpecial('~') {
				return dec.Err()
			}
		default:
			return newClientBugError("Unknown APPEND data extension")
		}
	}

	lit, nonSync, err := dec.ExpectLiteralReader()
	if err != nil {
		return err
	}

	appendLimit := int64(defaultAppendLimit)
	if appendLimitSession, ok := c.session.(SessionAppendLimit); ok {
		appendLimit = int64(appendLimitSession.AppendLimit())
	}

	if lit.Size() > appendLimit {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeTooBig,
			Text: fmt.Sprintf("Literals are limited to %v bytes for this command", appendLimit),
		}
	}
	if err := c.acceptLiteral(lit.Size(), nonSync); err != nil {
		return err
	}

	c.setReadTimeout(literalReadTimeout)
	defer c.setReadTimeout(cmdReadTimeout)

	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		io.Copy(io.Discard, lit)
		dec.CRLF()
		return err
	}

	data, appendErr := c.session.Append(mailbox, lit, &options)
	if _, discardErr := io.Copy(io.Discard, lit); discardErr != nil {
		return discardErr
	}
	// The refusal wins over the tail: if the session refused (ACL, quota, ...),
	// nothing was stored, so a malformed trailing CRLF is moot and the refusal
	// is the truthful answer.
	if appendErr != nil {
		return appendErr
	}
	// The store happened. A malformed command tail -- an extra CR, which
	// openssl s_client -crlf makes of a client's explicit \r\n -- must NOT be
	// answered BAD here: the message IS stored, and RFC 9051 7.1.3 has BAD mean
	// the command did not run, so a client reads BAD, retries, and stores a
	// second copy (#1129). The trailing tokens are consumed best-effort; the
	// connection is resynced by the caller's DiscardLine, and OK is the truthful
	// answer to a store that succeeded.
	//
	// Deliberate consequence: a client whose framing is broken now gets OK and
	// never learns of its bug -- it hung before #1127, saw BAD after, and now
	// succeeds. That is the right trade (the alternative punishes the mailbox
	// for a client defect), but it means this server no longer surfaces a
	// client's framing fault on APPEND.
	if dataExt != "" {
		dec.ExpectSpecial(')')
	}
	dec.ExpectCRLF()
	if err := c.poll("APPEND"); err != nil {
		return err
	}
	return c.writeAppendOK(tag, data)
}

func (c *Conn) writeAppendOK(tag string, data *imap.AppendData) error {
	enc := newResponseEncoder(c)
	defer enc.end()

	enc.Atom(tag).SP().Atom("OK").SP()
	if data != nil {
		enc.Special('[')
		enc.Atom("APPENDUID").SP().Number(data.UIDValidity).SP().UID(data.UID)
		enc.Special(']').SP()
	}
	enc.Text("APPEND completed")
	return enc.CRLF()
}
