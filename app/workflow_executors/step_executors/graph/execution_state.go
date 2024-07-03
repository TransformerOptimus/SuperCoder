package graph

type ExecutionState int

const (
	ExecutionSuccessState ExecutionState = iota
	ExecutionErrorState
	ExecutionRetryState
)
