package cmd

type ArgumentError struct {
	error
}

type ValidationError struct {
	error
}

type InternalError struct {
	error
}
