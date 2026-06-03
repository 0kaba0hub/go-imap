package imapserver

import "github.com/emersion/go-imap/v2"

// SessionQuota is an IMAP session which supports the QUOTA extension
// (RFC 9208). Implementing this interface on the session causes the
// server to advertise the "QUOTA" capability and dispatch
// GETQUOTAROOT and GETQUOTA commands to the session.
//
// SETQUOTA is always rejected with NOPERM — quota limits are admin-
// managed and cannot be changed by clients via IMAP.
type SessionQuota interface {
	Session

	// GetQuotaRoot returns quota data for all roots that apply to
	// the named mailbox. The returned QuotaRootData carries both the
	// root name list (for the QUOTAROOT untagged response) and the
	// per-root usage+limit data (for the QUOTA untagged responses).
	GetQuotaRoot(mailbox string) (*imap.QuotaRootData, error)

	// GetQuota returns the current usage and limits for the named
	// quota root. Returns an error if the root is unknown.
	GetQuota(root string) (*imap.QuotaData, error)
}
