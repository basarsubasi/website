package validation

type Error struct{ Message string }

func (e *Error) Error() string     { return e.Message }
func Invalid(message string) error { return &Error{Message: message} }
