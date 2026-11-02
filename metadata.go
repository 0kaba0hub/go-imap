package imap

import "fmt"

// GetMetadataDepth represents the depth parameter for GETMETADATA command.
type GetMetadataDepth int

const (
	GetMetadataDepthZero     GetMetadataDepth = 0
	GetMetadataDepthOne      GetMetadataDepth = 1
	GetMetadataDepthInfinity GetMetadataDepth = -1
)

// String returns the string representation of the depth value.
func (depth GetMetadataDepth) String() string {
	switch depth {
	case GetMetadataDepthZero:
		return "0"
	case GetMetadataDepthOne:
		return "1"
	case GetMetadataDepthInfinity:
		return "infinity"
	default:
		panic(fmt.Errorf("imap: unknown GETMETADATA depth %d", depth))
	}
}

// GetMetadataOptions contains options for the GETMETADATA command.
type GetMetadataOptions struct {
	MaxSize *uint32
	Depth   GetMetadataDepth
}

// GetMetadataData is the data returned by the GETMETADATA command.
type GetMetadataData struct {
	Mailbox          string
	Entries          map[string]*[]byte
	ResponseCodeData *MetadataResponseCodeData // Response code data from server (e.g., LONGENTRIES size)
}

// MetadataResponseCodeData contains data for METADATA-specific response codes.
type MetadataResponseCodeData struct {
	// Size is used with LONGENTRIES and MAXSIZE response codes
	Size uint32
}

// ValidateMetadataEntry validates a metadata entry name according to RFC 5464.
// Entry names must:
// - Be a scope root (/private, /shared) or start with /private/ or /shared/
// - Not contain * or %
// - Not contain consecutive slashes
// - Not end with a slash (unless it's just the prefix)
func ValidateMetadataEntry(entry string) error {
	if entry == "" {
		return fmt.Errorf("empty entry name")
	}

	// A scope root names the whole scope, which GETMETADATA with DEPTH reads
	if IsMetadataScopeRoot(entry) {
		return nil
	}

	// Must start with /private/ or /shared/
	if !hasPrefix(entry, "/private/") && !hasPrefix(entry, "/shared/") {
		return fmt.Errorf("entry name must start with /private/ or /shared/")
	}

	// Cannot contain wildcards
	if contains(entry, "*") || contains(entry, "%") {
		return fmt.Errorf("entry name cannot contain wildcards")
	}

	// Cannot have consecutive slashes
	if contains(entry, "//") {
		return fmt.Errorf("entry name cannot contain consecutive slashes")
	}

	// Cannot end with slash (except for the base /private/ or /shared/)
	if entry != "/private/" && entry != "/shared/" && hasSuffix(entry, "/") {
		return fmt.Errorf("entry name cannot end with a slash")
	}

	return nil
}

// IsMetadataScopeRoot reports whether entry names a whole scope rather than
// one entry: /private or /shared, with or without the trailing slash. Such a
// name can be read with DEPTH but holds no value of its own to set.
func IsMetadataScopeRoot(entry string) bool {
	switch entry {
	case "/private", "/shared", "/private/", "/shared/":
		return true
	}
	return false
}

// Helper functions to avoid importing strings package
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
