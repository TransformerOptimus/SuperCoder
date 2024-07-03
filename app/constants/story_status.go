package constants

const (
	Todo                    = "TODO"
	InProgress              = "IN_PROGRESS"
	Done                    = "DONE"
	MaxLoopIterationReached = "MAX_LOOP_ITERATION_REACHED"
	InReviewLLMKeyNotFound  = "IN_REVIEW_LLM_KEY_NOT_FOUND"
	InReview                = "IN_REVIEW"
)

func ValidStatuses() map[string]bool {
	return map[string]bool{
		Todo:                    true,
		InProgress:              true,
		Done:                    true,
		MaxLoopIterationReached: true,
		InReviewLLMKeyNotFound:  true,
		InReview:                true,
	}
}
