package prompts

import _ "embed"

//go:embed system_review.txt
var SystemReview string

//go:embed max_turns.txt
var MaxTurns string

//go:embed audit.txt
var Audit string
