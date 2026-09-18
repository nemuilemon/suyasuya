package domain

type Error struct {
	Code    int
	Message string
}

func (e Error) Error() string      { return e.Message }
func Invalid(message string) error { return Error{400, message} }
