package rpc

import "github.com/sanketn26/gossipcache/internal/wire"

// StatusCarriesCommittedVersion reports whether a mutation response must carry
// the committed VersionTag. Write-confirm timeout is success-plus: the commit
// happened and the node must install the version before surfacing the error.
func StatusCarriesCommittedVersion(s wire.Status) bool {
	switch s {
	case wire.StatusOK, wire.StatusErrWriteConfirmTimeout:
		return true
	default:
		return false
	}
}

// StatusCarriesGetRecord reports whether a get response may carry version,
// kind, value, and TTL. Only StatusOK does; not-found, not-caught-up, and
// errors never carry record data.
func StatusCarriesGetRecord(s wire.Status) bool {
	return s == wire.StatusOK
}

// RetrySameMutationID reports whether a wire status leaves the mutation
// eligible for retry with the same MutationID. Terminal and committed-success
// statuses never retry the commit.
//
// Transport resets and connection loss are not wire statuses; the gRPC client
// layer classifies those failures and reuses the MutationID on its own retry
// path (same policy as StatusErrRateLimited).
func RetrySameMutationID(s wire.Status) bool {
	return s.Retryable()
}
