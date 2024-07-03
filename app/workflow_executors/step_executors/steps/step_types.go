package steps

type StepType string

var (
	GIT            StepType = "GIT"
	LLM            StepType = "LLM"
	CODE_TEST      StepType = "CODE_TEST"
	FILE_OPERATION StepType = "FILE_OPERATION"
	COMMAND        StepType = "COMMAND"
)

func (s StepType) String() string {
	return string(s)
}
