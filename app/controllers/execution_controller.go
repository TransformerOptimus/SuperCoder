package controllers

import (
	"ai-developer/app/services"
)

type ExecutionController struct {
	Service *services.ExecutionService
}

func NewExecutionController(service *services.ExecutionService) *ExecutionController {
	return &ExecutionController{Service: service}
}
