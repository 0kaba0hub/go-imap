package imap

// ThreadAlgorithm is a threading algorithm.
type ThreadAlgorithm string

const (
	ThreadOrderedSubject ThreadAlgorithm = "ORDEREDSUBJECT"
	ThreadReferences     ThreadAlgorithm = "REFERENCES"
)

// ThreadCap returns the capability name announcing support for alg
// (RFC 5256 §3): a server states which algorithms it implements, one
// capability each.
func ThreadCap(alg ThreadAlgorithm) Cap {
	return Cap("THREAD=" + alg)
}

// ThreadNode is one node of a THREAD reply tree (RFC 5256 §4).
//
// Num is a sequence number or a UID, matching the command that asked. A zero
// Num is the "dummy" of §3: a message the thread needs to hold its children
// together but which is not in the mailbox -- a reply whose parent was never
// delivered, or was deleted. It carries no number on the wire, only its
// children, which is why it cannot be represented as an ordinary node.
type ThreadNode struct {
	Num      uint32
	Children []ThreadNode
}
