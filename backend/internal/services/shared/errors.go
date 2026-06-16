package shared

// ErrorData wraps a human-readable message in the standard error payload shape
// used by service handlers.
func ErrorData(message string) map[string]any {
	return map[string]any{"message": message}
}
