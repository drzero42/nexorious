package storefront

// reasonError is a rejection that errors.Is its sentinel, and whose message
// is the reason alone, without the sentinel's: the handler words the
// response around it.
type reasonError struct{ sentinel, reason error }

func (e reasonError) Error() string   { return e.reason.Error() }
func (e reasonError) Unwrap() []error { return []error{e.sentinel, e.reason} }
