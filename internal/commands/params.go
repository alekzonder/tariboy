package commands

// stringSlice coerces a JSON value into a slice of strings. It is shared by
// commands that accept a JSON list with no registry slice argument type.
func stringSlice(v any) ([]string, bool) {
	switch t := v.(type) {
	case nil:
		return nil, true
	case []string:
		return t, true
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			value, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, value)
		}
		return out, true
	default:
		return nil, false
	}
}
