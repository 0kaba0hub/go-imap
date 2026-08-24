package imapserver

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/internal/imapwire"
	"github.com/emersion/go-sasl"
)

var errAuthFailed = &imap.Error{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeAuthenticationFailed,
	Text: "Authentication failed",
}

// ErrAuthFailed is returned by Session.Login on authentication failure.
var ErrAuthFailed = errAuthFailed

// GreetingData is the data associated with an IMAP greeting.
type GreetingData struct {
	PreAuth bool
}

// NumKind describes how a number should be interpreted: either as a sequence
// number, either as a UID.
type NumKind int

const (
	NumKindSeq = NumKind(imapwire.NumKindSeq)
	NumKindUID = NumKind(imapwire.NumKindUID)
)

// String implements fmt.Stringer.
func (kind NumKind) String() string {
	switch kind {
	case NumKindSeq:
		return "seq"
	case NumKindUID:
		return "uid"
	default:
		panic(fmt.Errorf("imapserver: unknown NumKind %d", kind))
	}
}

func (kind NumKind) wire() imapwire.NumKind {
	return imapwire.NumKind(kind)
}

// Session is an IMAP session.
type Session interface {
	Close() error

	// Not authenticated state
	Login(username, password string) error

	// Authenticated state
	Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error)
	Create(mailbox string, options *imap.CreateOptions) error
	Delete(mailbox string) error
	Rename(mailbox, newName string, options *imap.RenameOptions) error
	Subscribe(mailbox string) error
	Unsubscribe(mailbox string) error
	List(w *ListWriter, ref string, patterns []string, options *imap.ListOptions) error
	Status(mailbox string, options *imap.StatusOptions) (*imap.StatusData, error)
	Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error)
	Poll(w *UpdateWriter, allowExpunge bool) error
	Idle(w *UpdateWriter, stop <-chan struct{}) error

	// Selected state
	Unselect() error
	Expunge(w *ExpungeWriter, uids *imap.UIDSet) error
	Search(kind NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error)
	Fetch(w *FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error
	Store(w *FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error
	Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error)
}

// SessionNamespace is an IMAP session which supports NAMESPACE.
type SessionNamespace interface {
	Session

	// Authenticated state
	Namespace() (*imap.NamespaceData, error)
}

// SessionMove is an IMAP session which supports MOVE.
type SessionMove interface {
	Session

	// Selected state
	Move(w *MoveWriter, numSet imap.NumSet, dest string) error
}

// SessionThread is an IMAP session which supports THREAD (RFC 5256).
//
// Threading is computed by the session, not by this package: the tree depends
// on headers and on the base subject rules of §2.1, both of which only the
// backend can see.
type SessionThread interface {
	Session

	// Selected state
	Thread(kind NumKind, alg imap.ThreadAlgorithm, criteria *imap.SearchCriteria) ([]imap.ThreadNode, error)
}

// SessionSort is an IMAP session which supports SORT (RFC 5256).
//
// The session returns the matching messages already ordered, because sorting
// needs the message data -- headers, sizes, dates -- that only the backend
// has. Numbers are sequence numbers or UIDs, according to kind.
type SessionSort interface {
	Session

	// Selected state
	Sort(kind NumKind, criteria []imap.SortCriterion, search *imap.SearchCriteria) ([]uint32, error)
}

// SessionNotify is an IMAP session which supports the NOTIFY extension
// (RFC 5465).
type SessionNotify interface {
	Session

	// Notify configures which unsolicited responses the session sends for
	// mailbox events. options is nil for NOTIFY NONE (disable). For NOTIFY SET
	// with the STATUS option the session may write immediate STATUS responses
	// for the newly monitored mailboxes via w.
	Notify(w *UpdateWriter, options *imap.NotifyOptions) error
}

// SessionIMAP4rev2 is an IMAP session which supports IMAP4rev2.
type SessionIMAP4rev2 interface {
	Session
	SessionNamespace
	SessionMove
}

// SessionSASL is an IMAP session which supports its own set of SASL
// authentication mechanisms.
type SessionSASL interface {
	Session
	AuthenticateMechanisms() []string
	Authenticate(mech string) (sasl.Server, error)
}

// SessionUnauthenticate is an IMAP session which supports UNAUTHENTICATE.
type SessionUnauthenticate interface {
	Session

	// Authenticated state
	Unauthenticate() error
}

// SessionAppendLimit is an IMAP session which has the same APPEND limit for
// all mailboxes.
type SessionAppendLimit interface {
	Session

	// AppendLimit returns the maximum size in bytes that can be uploaded to
	// this server in an APPEND command.
	AppendLimit() uint32
}

// SessionCapabilities is an IMAP session which can provide its current
// capabilities for capability filtering.
type SessionCapabilities interface {
	Session

	// GetCapabilities returns the session-specific capabilities.
	// This allows sessions to filter capabilities based on client behavior
	// or other session-specific factors.
	GetCapabilities() imap.CapSet
}

// SessionMetadata is an IMAP session which supports the METADATA extension (RFC 5464).
type SessionMetadata interface {
	Session

	// GetMetadata retrieves server or mailbox annotations.
	// If mailbox is empty string "", retrieve server annotations.
	// entries contains the list of entry names to retrieve.
	GetMetadata(mailbox string, entries []string, options *imap.GetMetadataOptions) (*imap.GetMetadataData, error)

	// SetMetadata sets or removes server or mailbox annotations.
	// If mailbox is empty string "", set server annotations.
	// To remove an entry, set its value to nil.
	SetMetadata(mailbox string, entries map[string]*[]byte) error
}
