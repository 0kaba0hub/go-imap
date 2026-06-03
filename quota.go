package imap

// QuotaResourceType is a QUOTA resource type.
//
// See RFC 9208 section 5.
type QuotaResourceType string

const (
	QuotaResourceStorage           QuotaResourceType = "STORAGE"
	QuotaResourceMessage           QuotaResourceType = "MESSAGE"
	QuotaResourceMailbox           QuotaResourceType = "MAILBOX"
	QuotaResourceAnnotationStorage QuotaResourceType = "ANNOTATION-STORAGE"
)

// QuotaResource is one resource entry within a quota root (RFC 9208 §3).
// Usage and Limit for STORAGE are in kibibytes (1 KiB = 1024 bytes) per RFC;
// for MESSAGE they are message counts.
type QuotaResource struct {
	Type  QuotaResourceType
	Usage uint64
	Limit uint64 // 0 = no limit
}

// QuotaData is the data for one quota root — returned in untagged QUOTA
// responses.
type QuotaData struct {
	Name      string         // quota root name (e.g. "User quota")
	Resources []QuotaResource
}

// QuotaRootData is the combined server response to GETQUOTAROOT: the list
// of applicable quota roots for the mailbox, plus the current resource
// usage and limits for each root.
type QuotaRootData struct {
	Mailbox string
	Roots   []string    // quota root names (the QUOTAROOT untagged response)
	Quotas  []QuotaData // one QuotaData per root (the QUOTA untagged responses)
}
