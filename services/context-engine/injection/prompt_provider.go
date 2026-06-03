package injection

import (
	serviceimpl "github.com/TransformerOptimus/SuperCoder/services/context-engine/services/impl"
)

func ProvidePromptProvider() *serviceimpl.PromptProvider {
	return serviceimpl.NewPromptProvider()
}
