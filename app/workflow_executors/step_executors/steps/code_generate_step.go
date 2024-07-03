package steps

type GenerateCodeStep struct {
	BaseStep
	WorkflowStep
	Retry             bool
	MaxLoopIterations int64
	PullRequestID     uint
}

func (s GenerateCodeStep) StepType() string {
	return LLM.String()
}

func (s GenerateCodeStep) StepName() string {
	return CODE_GENERATE_STEP.String()
}

func (s *GenerateCodeStep) WithPullRequestID(pullRequestID uint) {
	s.PullRequestID = pullRequestID
}
