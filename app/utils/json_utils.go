package utils

import (
	"ai-developer/app/config"
	"ai-developer/app/models/types"
	"encoding/json"
	"go.uber.org/zap"
)

func GetAsJsonMap(item interface{}) (jsonMap *types.JSONMap, err error) {
	jsonBytes, err := json.Marshal(item)
	if err != nil {
		config.Logger.Error("Error marshalling item", zap.Error(err))
		return
	}
	err = json.Unmarshal(jsonBytes, &jsonMap)
	if err != nil {
		config.Logger.Error("Error unmarshalling item", zap.Error(err))
		return
	}
	return
}
