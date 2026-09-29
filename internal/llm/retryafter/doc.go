// Package retryafter provides shared handling for provider Retry-After hints.
//
// Provider runtimes keep their own retry classification and request logic,
// but parsing an untrusted server delay and carrying it through an error
// should be consistent. Delays are capped so a malformed or hostile upstream
// cannot stall a turn indefinitely.
package retryafter
