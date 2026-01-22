package protect

import "strings"

// matchGlobPattern matches a path against a glob pattern with ** support.
func matchGlobPattern(path, pattern string) bool {
	pathParts := strings.Split(path, "/")
	patternParts := strings.Split(pattern, "/")
	return matchParts(pathParts, patternParts)
}

// matchParts recursively matches path segments against pattern segments.
func matchParts(path, pattern []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}

	p := pattern[0]
	rest := pattern[1:]

	switch p {
	case "**":
		if len(rest) == 0 {
			return true
		}
		for i := 0; i <= len(path); i++ {
			if matchParts(path[i:], rest) {
				return true
			}
		}
		return false

	default:
		if len(path) == 0 {
			return false
		}
		if !matchSegment(path[0], p) {
			return false
		}
		return matchParts(path[1:], rest)
	}
}

// matchSegment matches a single path segment against a pattern segment.
func matchSegment(segment, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == segment {
		return true
	}
	if strings.Contains(pattern, "*") {
		return matchWildcard(segment, pattern)
	}
	return false
}

// matchWildcard matches a segment against a pattern containing * wildcards.
func matchWildcard(s, pattern string) bool {
	parts := strings.Split(pattern, "*")
	pos := 0

	for i, part := range parts {
		if part == "" {
			continue
		}

		if i == 0 {
			if !strings.HasPrefix(s, part) {
				return false
			}
			pos = len(part)
			continue
		}

		if i == len(parts)-1 && !strings.HasSuffix(pattern, "*") {
			if !strings.HasSuffix(s, part) {
				return false
			}
			continue
		}

		idx := strings.Index(s[pos:], part)
		if idx == -1 {
			return false
		}
		pos += idx + len(part)
	}

	return true
}
