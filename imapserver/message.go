package imapserver

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-message/textproto"

	"github.com/emersion/go-imap/v2"
)

// ExtractBodySection extracts a section of a message body.
//
// It can be used by server backends to implement Session.Fetch.
func ExtractBodySection(r io.Reader, item *imap.FetchItemBodySection) []byte {
	var (
		header textproto.Header
		body   io.Reader
	)

	// The bytes read while parsing the header are kept, so a header the parser
	// refuses can still be served from the wire form without re-reading a
	// stream that cannot be rewound. The normal path stays streaming: nothing
	// but the header is buffered.
	var seen bytes.Buffer
	br := bufio.NewReader(io.TeeReader(r, &seen))
	header, err := textproto.ReadHeader(br)
	if err != nil {
		// A message whose header the parser rejects -- a line without a colon
		// is the common one, and such mail does arrive -- must not become
		// unfetchable. Answering an empty section tells the client the message
		// is there and gives it nothing, which is worse than serving what was
		// stored. Fall back to the wire's own rule: the header ends at the
		// first empty line.
		rest, restErr := io.ReadAll(r)
		if restErr != nil {
			return nil
		}
		return extractLenient(append(seen.Bytes(), rest...), item)
	}
	body = br

	parentMediaType, header, body := findMessagePart(header, body, item.Part)
	if body == nil {
		return nil
	}

	if len(item.Part) > 0 {
		switch item.Specifier {
		case imap.PartSpecifierHeader, imap.PartSpecifierText:
			header, body = openMessagePart(header, body, parentMediaType)
		}
	}

	// Filter header fields
	if len(item.HeaderFields) > 0 {
		keep := make(map[string]struct{})
		for _, k := range item.HeaderFields {
			keep[strings.ToLower(k)] = struct{}{}
		}
		for field := header.Fields(); field.Next(); {
			if _, ok := keep[strings.ToLower(field.Key())]; !ok {
				field.Del()
			}
		}
	}
	for _, k := range item.HeaderFieldsNot {
		header.Del(k)
	}

	// Write the requested data to a buffer
	var buf bytes.Buffer

	writeHeader := true
	switch item.Specifier {
	case imap.PartSpecifierNone:
		writeHeader = len(item.Part) == 0
	case imap.PartSpecifierText:
		writeHeader = false
	}
	if writeHeader {
		if err := textproto.WriteHeader(&buf, header); err != nil {
			return nil
		}
	}

	switch item.Specifier {
	case imap.PartSpecifierNone, imap.PartSpecifierText:
		if _, err := io.Copy(&buf, body); err != nil {
			return nil
		}
	}

	return extractPartial(buf.Bytes(), item.Partial)
}

func findMessagePart(header textproto.Header, body io.Reader, partPath []int) (string, textproto.Header, io.Reader) {
	// First part of non-multipart message refers to the message itself
	msgHeader := gomessage.Header{header}
	mediaType, _, _ := msgHeader.ContentType()
	if !strings.HasPrefix(mediaType, "multipart/") && len(partPath) > 0 && partPath[0] == 1 {
		partPath = partPath[1:]
	}

	var parentMediaType string
	for i := 0; i < len(partPath); i++ {
		partNum := partPath[i]

		header, body = openMessagePart(header, body, parentMediaType)

		msgHeader := gomessage.Header{header}
		mediaType, typeParams, _ := msgHeader.ContentType()
		if !strings.HasPrefix(mediaType, "multipart/") {
			if partNum != 1 {
				return "", textproto.Header{}, nil
			}
			continue
		}

		mr := textproto.NewMultipartReader(body, typeParams["boundary"])
		found := false
		for j := 1; j <= partNum; j++ {
			p, err := mr.NextPart()
			if err != nil {
				return "", textproto.Header{}, nil
			}

			if j == partNum {
				parentMediaType = mediaType
				header = p.Header
				body = p
				found = true
				break
			}
		}
		if !found {
			return "", textproto.Header{}, nil
		}
	}

	return parentMediaType, header, body
}

func openMessagePart(header textproto.Header, body io.Reader, parentMediaType string) (textproto.Header, io.Reader) {
	msgHeader := gomessage.Header{header}
	mediaType, _, _ := msgHeader.ContentType()
	if !msgHeader.Has("Content-Type") && parentMediaType == "multipart/digest" {
		mediaType = "message/rfc822"
	}
	if mediaType == "message/rfc822" || mediaType == "message/global" {
		br := bufio.NewReader(body)
		header, _ = textproto.ReadHeader(br)
		return header, br
	}
	return header, body
}

func extractPartial(b []byte, partial *imap.SectionPartial) []byte {
	if partial == nil {
		return b
	}

	if partial.Offset > int64(len(b)) {
		return nil
	}
	// Offset+Size can overflow int64, so clamp the remaining length instead.
	size := int64(len(b)) - partial.Offset
	if partial.Size >= 0 && partial.Size < size {
		size = partial.Size
	}
	return b[partial.Offset : partial.Offset+size]
}

func ExtractBinarySection(r io.Reader, item *imap.FetchItemBinarySection) []byte {
	var (
		header textproto.Header
		body   io.Reader
	)

	// As in ExtractBodySection: what the header parser read is kept, so a
	// header it refuses does not cost the client the message.
	var seen bytes.Buffer
	br := bufio.NewReader(io.TeeReader(r, &seen))
	header, err := textproto.ReadHeader(br)
	if err != nil {
		if len(item.Part) > 0 {
			// A named part needs a parse, and a decoded part needs its
			// Content-Transfer-Encoding, which is exactly what could not be
			// read. Nothing honest to return.
			return nil
		}
		// No part, no encoding to trust: serve the message as stored.
		rest, restErr := io.ReadAll(r)
		if restErr != nil {
			return nil
		}
		return append(seen.Bytes(), rest...)
	}
	body = br

	_, header, body = findMessagePart(header, body, item.Part)
	if body == nil {
		return nil
	}

	part, err := gomessage.New(gomessage.Header{header}, body)
	if err != nil {
		return nil
	}

	// Write the requested data to a buffer
	var buf bytes.Buffer

	if len(item.Part) == 0 {
		if err := textproto.WriteHeader(&buf, part.Header.Header); err != nil {
			return nil
		}
	}

	if _, err := io.Copy(&buf, part.Body); err != nil {
		return nil
	}

	return extractPartial(buf.Bytes(), item.Partial)
}

func ExtractBinarySectionSize(r io.Reader, item *imap.FetchItemBinarySectionSize) uint32 {
	// TODO: optimize
	b := ExtractBinarySection(r, &imap.FetchItemBinarySection{Part: item.Part})
	return uint32(len(b))
}

// ExtractEnvelope returns a message envelope from its header.
//
// It can be used by server backends to implement Session.Fetch.
func ExtractEnvelope(h textproto.Header) *imap.Envelope {
	mh := mail.Header{gomessage.Header{h}}
	date, _ := mh.Date()
	subject, _ := mh.Subject()
	inReplyTo, _ := mh.MsgIDList("In-Reply-To")
	messageID, _ := mh.MessageID()
	return &imap.Envelope{
		Date:      date,
		Subject:   subject,
		From:      parseAddressList(mh, "From"),
		Sender:    parseAddressList(mh, "Sender"),
		ReplyTo:   parseAddressList(mh, "Reply-To"),
		To:        parseAddressList(mh, "To"),
		Cc:        parseAddressList(mh, "Cc"),
		Bcc:       parseAddressList(mh, "Bcc"),
		InReplyTo: inReplyTo,
		MessageID: messageID,
	}
}

func parseAddressList(mh mail.Header, k string) []imap.Address {
	// TODO: handle groups
	addrs, _ := mh.AddressList(k)
	var l []imap.Address
	for _, addr := range addrs {
		mailbox, host, ok := strings.Cut(addr.Address, "@")
		if !ok {
			continue
		}
		l = append(l, imap.Address{
			Name:    addr.Name,
			Mailbox: mailbox,
			Host:    host,
		})
	}
	return l
}

// ExtractBodyStructure extracts the structure of a message body.
//
// It can be used by server backends to implement Session.Fetch.
func ExtractBodyStructure(r io.Reader) imap.BodyStructure {
	br := bufio.NewReader(r)
	header, _ := textproto.ReadHeader(br)
	return extractBodyStructure(header, br)
}

func extractBodyStructure(rawHeader textproto.Header, r io.Reader) imap.BodyStructure {
	header := gomessage.Header{rawHeader}

	mediaType, typeParams, _ := header.ContentType()
	primaryType, subType, _ := strings.Cut(mediaType, "/")

	if primaryType == "multipart" {
		bs := &imap.BodyStructureMultiPart{Subtype: subType}
		mr := textproto.NewMultipartReader(r, typeParams["boundary"])
		for {
			part, _ := mr.NextPart()
			if part == nil {
				break
			}
			bs.Children = append(bs.Children, extractBodyStructure(part.Header, part))
		}
		bs.Extended = &imap.BodyStructureMultiPartExt{
			Params:      typeParams,
			Disposition: getContentDisposition(header),
			Language:    getContentLanguage(header),
			Location:    header.Get("Content-Location"),
		}
		return bs
	} else {
		body, _ := io.ReadAll(r) // TODO: optimize
		bs := &imap.BodyStructureSinglePart{
			Type:        primaryType,
			Subtype:     subType,
			Params:      typeParams,
			ID:          header.Get("Content-Id"),
			Description: header.Get("Content-Description"),
			Encoding:    header.Get("Content-Transfer-Encoding"),
			Size:        uint32(len(body)),
		}
		if mediaType == "message/rfc822" || mediaType == "message/global" {
			br := bufio.NewReader(bytes.NewReader(body))
			childHeader, _ := textproto.ReadHeader(br)
			bs.MessageRFC822 = &imap.BodyStructureMessageRFC822{
				Envelope:      ExtractEnvelope(childHeader),
				BodyStructure: extractBodyStructure(childHeader, br),
				NumLines:      int64(bytes.Count(body, []byte("\n"))),
			}
		}
		if primaryType == "text" {
			bs.Text = &imap.BodyStructureText{
				NumLines: int64(bytes.Count(body, []byte("\n"))),
			}
		}
		bs.Extended = &imap.BodyStructureSinglePartExt{
			Disposition: getContentDisposition(header),
			Language:    getContentLanguage(header),
			Location:    header.Get("Content-Location"),
		}
		return bs
	}
}

func getContentDisposition(header gomessage.Header) *imap.BodyStructureDisposition {
	disp, dispParams, _ := header.ContentDisposition()
	if disp == "" {
		return nil
	}
	return &imap.BodyStructureDisposition{
		Value:  disp,
		Params: dispParams,
	}
}

func getContentLanguage(header gomessage.Header) []string {
	v := header.Get("Content-Language")
	if v == "" {
		return nil
	}
	// TODO: handle CFWS
	l := strings.Split(v, ",")
	for i, lang := range l {
		l[i] = strings.TrimSpace(lang)
	}
	return l
}

// extractLenient serves a section of a message whose header the strict parser
// would not read. It splits at the first empty line and nothing more: that is
// the only structure such a message reliably has.
//
// A section naming a MIME part is not served this way and comes back nil --
// walking parts needs a parse, and inventing one would answer a client's
// precise question with a guess.
func extractLenient(raw []byte, item *imap.FetchItemBodySection) []byte {
	if len(item.Part) > 0 {
		return nil
	}
	head, body := splitHeader(raw)

	switch item.Specifier {
	case imap.PartSpecifierNone:
		if len(item.HeaderFields) == 0 && len(item.HeaderFieldsNot) == 0 {
			return raw
		}
	case imap.PartSpecifierText:
		return body
	case imap.PartSpecifierHeader, imap.PartSpecifierMIME:
		if len(item.HeaderFields) == 0 && len(item.HeaderFieldsNot) == 0 {
			return head
		}
	default:
		return nil
	}
	return filterFieldsLenient(head, item)
}

// splitHeader cuts a message at the first empty line. The header keeps its
// terminating blank line, as BODY[HEADER] must (RFC 9051 6.4.5).
func splitHeader(raw []byte) (head, body []byte) {
	for _, sep := range [][]byte{[]byte("\r\n\r\n"), []byte("\n\n")} {
		if i := bytes.Index(raw, sep); i >= 0 {
			return raw[:i+len(sep)], raw[i+len(sep):]
		}
	}
	return raw, nil
}

// filterFieldsLenient applies HEADER.FIELDS / HEADER.FIELDS.NOT by walking the
// header block line by line. A line that names no field is carried with the
// field above it, which is what a folded value looks like -- and what the
// malformed line that brought us here looks like too.
func filterFieldsLenient(head []byte, item *imap.FetchItemBodySection) []byte {
	keep := make(map[string]struct{}, len(item.HeaderFields))
	for _, k := range item.HeaderFields {
		keep[strings.ToLower(k)] = struct{}{}
	}
	drop := make(map[string]struct{}, len(item.HeaderFieldsNot))
	for _, k := range item.HeaderFieldsNot {
		drop[strings.ToLower(k)] = struct{}{}
	}

	var out bytes.Buffer
	keeping := false
	for _, line := range splitLinesKeepEnding(head) {
		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) == 0 {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			if keeping {
				out.Write(line)
			}
			continue
		}
		name := ""
		if i := bytes.IndexByte(trimmed, ':'); i > 0 {
			name = strings.ToLower(string(trimmed[:i]))
		}
		if len(item.HeaderFields) > 0 {
			_, keeping = keep[name]
		} else {
			_, dropped := drop[name]
			keeping = !dropped
		}
		if keeping {
			out.Write(line)
		}
	}
	out.WriteString("\r\n")
	return out.Bytes()
}

func splitLinesKeepEnding(b []byte) [][]byte {
	var lines [][]byte
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			lines = append(lines, b)
			break
		}
		lines = append(lines, b[:i+1])
		b = b[i+1:]
	}
	return lines
}
