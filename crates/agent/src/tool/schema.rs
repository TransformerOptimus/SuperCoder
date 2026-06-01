use serde_json::Value;

/// Lightweight validation of tool arguments against a JSON schema.
///
/// Checks that required fields are present and have the correct basic type.
/// Returns a human-readable error message on failure (the LLM can learn from it).
pub fn validate_args(args: &Value, schema: &Value) -> Result<(), String> {
    let args_obj = args
        .as_object()
        .ok_or_else(|| "Arguments must be a JSON object".to_string())?;

    let schema_obj = match schema.as_object() {
        Some(o) => o,
        None => return Ok(()), // no schema = no validation
    };

    // Check required fields
    if let Some(Value::Array(required)) = schema_obj.get("required") {
        for req in required {
            if let Some(field_name) = req.as_str() {
                if !args_obj.contains_key(field_name) {
                    return Err(format!("Missing required parameter: '{field_name}'"));
                }
            }
        }
    }

    // Check types for provided fields
    if let Some(Value::Object(properties)) = schema_obj.get("properties") {
        for (key, value) in args_obj {
            if let Some(prop_schema) = properties.get(key) {
                if let Some(expected_type) = prop_schema.get("type").and_then(|t| t.as_str()) {
                    let type_ok = match expected_type {
                        "string" => value.is_string(),
                        "integer" => value.is_i64() || value.is_u64(),
                        "number" => value.is_number(),
                        "boolean" => value.is_boolean(),
                        "object" => value.is_object(),
                        "array" => value.is_array(),
                        _ => true, // unknown type, skip
                    };
                    if !type_ok {
                        return Err(format!(
                            "Parameter '{key}' must be of type '{expected_type}', got {}",
                            json_type_name(value)
                        ));
                    }
                }
            }
        }
    }

    Ok(())
}

fn json_type_name(v: &Value) -> &'static str {
    match v {
        Value::Null => "null",
        Value::Bool(_) => "boolean",
        Value::Number(_) => "number",
        Value::String(_) => "string",
        Value::Array(_) => "array",
        Value::Object(_) => "object",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_valid_args_pass() {
        let schema = json!({
            "type": "object",
            "required": ["filePath"],
            "properties": {
                "filePath": { "type": "string" },
                "offset": { "type": "integer" }
            }
        });
        let args = json!({ "filePath": "/test.rs", "offset": 10 });
        assert!(validate_args(&args, &schema).is_ok());
    }

    #[test]
    fn test_missing_required_field() {
        let schema = json!({
            "type": "object",
            "required": ["filePath"],
            "properties": {
                "filePath": { "type": "string" }
            }
        });
        let args = json!({});
        let err = validate_args(&args, &schema).unwrap_err();
        assert!(err.contains("filePath"));
    }

    #[test]
    fn test_type_mismatch() {
        let schema = json!({
            "type": "object",
            "required": ["filePath"],
            "properties": {
                "filePath": { "type": "string" }
            }
        });
        let args = json!({ "filePath": 42 });
        let err = validate_args(&args, &schema).unwrap_err();
        assert!(err.contains("string"));
    }

    #[test]
    fn test_no_schema_passes() {
        let args = json!({ "anything": "goes" });
        assert!(validate_args(&args, &json!(null)).is_ok());
    }

    #[test]
    fn test_args_not_object() {
        let schema = json!({ "type": "object" });
        let err = validate_args(&json!("not an object"), &schema).unwrap_err();
        assert!(err.contains("JSON object"));
    }
}
