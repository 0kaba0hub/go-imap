package imapserver

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
)

// handleThread implements THREAD and UID THREAD (RFC 5256 §4).
//
//	thread = ["UID" SP] "THREAD" SP thread-alg SP search-charset SP search-program
//
// Unlike SEARCH, the charset is not optional here: the grammar has no form
// without it, so a client that omits it gets a parse error rather than a
// silent US-ASCII assumption.
func (c *Conn) handleThread(dec *imapwire.Decoder, numKind NumKind) error {
	session, ok := c.session.(SessionThread)
	if !ok {
		return newClientBugError("THREAD is not supported")
	}

	var alg, charset string
	if !dec.ExpectSP() || !dec.ExpectAtom(&alg) || !dec.ExpectSP() || !dec.ExpectAString(&charset) || !dec.ExpectSP() {
		return dec.Err()
	}
	switch strings.ToUpper(charset) {
	case "US-ASCII", "UTF-8":
	default:
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeBadCharset,
			Text: "Only US-ASCII and UTF-8 are supported THREAD charsets",
		}
	}

	var criteria imap.SearchCriteria
	for {
		if err := readSearchKey(&criteria, dec); err != nil {
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

	// An algorithm the server does not implement is refused here rather than
	// in the session: it is answered from the capabilities the server itself
	// announced, so no backend can disagree with what the client was told.
	algorithm := imap.ThreadAlgorithm(strings.ToUpper(alg))
	if !c.server.options.caps().Has(imap.ThreadCap(algorithm)) {
		return &imap.Error{
			Type: imap.StatusResponseTypeBad,
			Text: fmt.Sprintf("Unsupported threading algorithm %q", alg),
		}
	}

	roots, err := session.Thread(numKind, algorithm, &criteria)
	if err != nil {
		return err
	}
	return c.writeThread(roots)
}

func (c *Conn) writeThread(roots []imap.ThreadNode) error {
	enc := newResponseEncoder(c)
	defer enc.end()

	writeThreadRoots(enc.Encoder, roots)
	return enc.CRLF()
}

// writeThreadRoots writes the whole untagged reply. One space separates the
// keyword from the first thread and nothing separates the threads from each
// other -- "* THREAD (2)(3 6)" -- so the separator belongs to the keyword, not
// to the lists.
func writeThreadRoots(enc *imapwire.Encoder, roots []imap.ThreadNode) {
	enc.Atom("*").SP().Atom("THREAD")
	for i, root := range roots {
		if i == 0 {
			enc.SP()
		}
		writeThreadNode(enc, root)
	}
}

// writeThreadNode writes one thread as the parenthesised list of RFC 5256 §4.
//
// The shape carries the tree: a chain of single-child descendants is flattened
// into a sequence, so "(1 2 3)" is a message, its reply, and the reply to
// that. Where a message has several replies, each becomes its own nested list
// -- "(1 2 (3)(4))" is two answers to message 2, not four messages in a row.
// A dummy node (§3) contributes only its parentheses, which is how a set of
// replies whose common parent is absent stays grouped: "((3)(5))".
func writeThreadNode(enc *imapwire.Encoder, node imap.ThreadNode) {
	enc.Special('(')
	writeThreadChain(enc, node, true)
	enc.Special(')')
}

func writeThreadChain(enc *imapwire.Encoder, node imap.ThreadNode, first bool) {
	for {
		if node.Num != 0 {
			if !first {
				enc.SP()
			}
			enc.Number(node.Num)
			first = false
		}
		switch len(node.Children) {
		case 0:
			return
		case 1:
			// Linear descent: stay in this list rather than nesting, so the
			// depth of the response follows branching and not conversation
			// length.
			node = node.Children[0]
		default:
			for _, child := range node.Children {
				if !first {
					// Exactly one space, before the first branch only: the
					// grammar separates the number from the branches, and the
					// branches from each other by nothing at all.
					enc.SP()
					first = true
				}
				writeThreadNode(enc, child)
			}
			return
		}
	}
}
