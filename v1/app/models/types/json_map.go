package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONMap is a custom type for handling JSON columns.
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface for JSONMap.
func (j JSONMap) Value() (driver.Value, error) {
	value, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	return value, nil
}

// Scan implements the sql.Scanner interface for JSONMap.
func (j *JSONMap) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(bytes, &j)
}
