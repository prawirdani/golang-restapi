package apperr

// Kind classifies an application error so transport layers can map it to a
// protocol-specific status (e.g. HTTP). Each Kind is a stable, exhaustive category.
type Kind uint8

const (
	KindValidation Kind = iota // input failed validation
	KindNotFound               // requested resource does not exist
	KindUnauthorized           // authentication missing or invalid
	KindConflict               // request conflicts with current state
	KindForbidden              // authenticated but not permitted
	KindThrottled              // rate limit exceeded
)
