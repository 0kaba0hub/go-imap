package imapserver

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

func (c *Conn) handleGetQuotaRoot(dec *imapwire.Decoder) error {
	var mailbox string
	if !dec.ExpectSP() || !dec.ExpectMailbox(&mailbox) || !dec.ExpectCRLF() {
		return dec.Err()
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}
	session, ok := c.session.(SessionQuota)
	if !ok {
		return newClientBugError("QUOTA extension is not supported")
	}
	data, err := session.GetQuotaRoot(mailbox)
	if err != nil {
		return err
	}
	// Emit: * QUOTAROOT <mailbox> <root1> [<root2> ...]
	if err := c.writeQuotaRoot(data); err != nil {
		return err
	}
	// Emit one: * QUOTA <root> (...) per root.
	for i := range data.Quotas {
		if err := c.writeQuota(&data.Quotas[i]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Conn) handleGetQuota(dec *imapwire.Decoder) error {
	var root string
	if !dec.ExpectSP() || !dec.ExpectAString(&root) || !dec.ExpectCRLF() {
		return dec.Err()
	}
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}
	session, ok := c.session.(SessionQuota)
	if !ok {
		return newClientBugError("QUOTA extension is not supported")
	}
	data, err := session.GetQuota(root)
	if err != nil {
		return err
	}
	return c.writeQuota(data)
}

// handleSetQuota always rejects — quota limits are not manageable via
// IMAP (mirrors the reference implementation).
func (c *Conn) handleSetQuota(dec *imapwire.Decoder) error {
	// Consume remainder of the command so the decoder stays in sync.
	dec.DiscardLine()
	if err := c.checkState(imap.ConnStateAuthenticated); err != nil {
		return err
	}
	return &imap.Error{
		Type: imap.StatusResponseTypeNo,
		Code: imap.ResponseCodeNoPerm,
		Text: "Permission denied",
	}
}

func (c *Conn) writeQuotaRoot(data *imap.QuotaRootData) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("QUOTAROOT").SP().Mailbox(data.Mailbox)
	for _, root := range data.Roots {
		enc.SP().String(root)
	}
	return enc.CRLF()
}

func (c *Conn) writeQuota(data *imap.QuotaData) error {
	enc := newResponseEncoder(c)
	defer enc.end()
	enc.Atom("*").SP().Atom("QUOTA").SP().String(data.Name).SP()
	enc.Special('(')
	for i := range data.Resources {
		r := &data.Resources[i]
		if i > 0 {
			enc.SP()
		}
		enc.Atom(string(r.Type)).SP().
			Atom(fmt.Sprintf("%d", r.Usage)).SP().
			Atom(fmt.Sprintf("%d", r.Limit))
	}
	enc.Special(')')
	return enc.CRLF()
}
