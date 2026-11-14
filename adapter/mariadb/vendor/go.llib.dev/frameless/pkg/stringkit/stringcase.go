// Package stringcase makes it simple to change the style of strings between formats like snake_case or PascalCase.
package stringkit

import (
	"strings"
	"unicode"
)

func IsSnake(s string) bool { return isSnakeKebab(s, '_') }

func ToSnake(s string) string {
	if IsSnake(s) {
		return s
	}
	const separator = '_'
	return toSnakeKebab(s, separator)
}

func IsScreamingSnake(s string) bool {
	return IsSnake(strings.ToLower(s)) && s == strings.ToUpper(s)
}

func ToScreamingSnake(s string) string {
	return strings.ToUpper(ToSnake(s))
}

func IsPascal(s string) bool {
	if len(s) == 0 {
		return false
	}

	// First character should be uppercase
	if !unicode.IsUpper([]rune(s)[0]) {
		return false
	}

	// Iterate through the string and check if it follows Pascal case rules
	for _, r := range s {
		if isSeparatorSymbol(r) {
			return false // No separators allowed
		}

		// Check if the current character is not a digit, a letter, or a valid Unicode letter
		if !unicode.IsDigit(r) && !unicode.IsLetter(r) && !unicode.Is(unicode.Letter, r) {
			return false
		}
	}

	return true
}

func ToPascal(s string) string {
	if IsPascal(s) {
		return s
	}

	if IsScreamingSnake(s) {
		s = strings.ToLower(s)
	}

	var (
		original = []rune(s)
		result   = make([]rune, 0, len(original))
		toUpper  = true
	)

	for i, r := range original {
		if isSeparatorSymbol(r) { // Convert spaces, dots, and underscores to uppercase flag
			toUpper = true
			continue
		}

		if unicode.IsUpper(r) {
			var prevChar, hasPrevChar = lookupPrevChar(original, i)
			if hasPrevChar && (unicode.IsLower(prevChar) || unicode.IsDigit(prevChar)) {
				toUpper = true
			} else if hasPrevChar && unicode.IsUpper(prevChar) {
				var nextChar, hasNextChar = lookupNextChar(original, i)
				if hasNextChar && unicode.IsLower(nextChar) {
					toUpper = true
				}
			}
		}

		if toUpper {
			result = append(result, unicode.ToUpper(r))
			toUpper = false
			continue
		}

		if nextOGChar, hasNextOGChar := lookupNextChar(original, i); hasNextOGChar && unicode.IsUpper(nextOGChar) && unicode.IsUpper(r) {
			result = append(result, r)
			continue
		}

		result = append(result, unicode.ToLower(r))
	}

	return string(result)
}

func IsCamel(s string) bool {
	if len(s) == 0 {
		return false
	}

	// First character should be uppercase
	if !unicode.IsLower([]rune(s)[0]) {
		return false
	}

	// Iterate through the string and check if it follows Pascal case rules
	for _, r := range s {
		if isSeparatorSymbol(r) {
			return false // No separators allowed
		}

		// Check if the current character is not a digit, a letter, or a valid Unicode letter
		if !unicode.IsDigit(r) && !unicode.IsLetter(r) && !unicode.Is(unicode.Letter, r) {
			return false
		}
	}

	return true
}

func ToCamel(s string) string {
	if IsCamel(s) {
		return s
	}
	if len(s) == 0 {
		return s
	}
	chars := []rune(ToPascal(s))
	for i, ch := range chars {
		chars[i] = unicode.ToLower(ch)

		nextCh, hasNext := lookupNextChar(chars, i)
		if !hasNext || unicode.IsLower(nextCh) {
			break
		}

		nextNextCh, hasNextNext := lookupNextChar(chars, i+1)
		if !hasNextNext || unicode.IsLower(nextNextCh) {
			break
		}
	}

	return string(chars)
}

func IsKebab(s string) bool {
	return isSnakeKebab(s, '-')
}

func ToKebab(s string) string {
	if IsKebab(s) {
		return s
	}
	const separator = '-'
	return toSnakeKebab(s, separator)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func isSnakeKebab(s string, separator rune) bool {
	var chars = []rune(s)

	if len(chars) == 0 {
		return false
	}

	// First and last characters should not be underscores
	if chars[0] == separator || chars[len(chars)-1] == separator {
		return false
	}

	// Iterate through the string and check if it follows snake case rules
	previousWasUnderscore := false
	for _, r := range chars {
		if r == separator {
			if previousWasUnderscore {
				return false // No consecutive underscores allowed
			}
			previousWasUnderscore = true
		} else {
			// Check if the current character is not a digit, a lowercase letter, or a valid Unicode lowercase letter
			if !unicode.IsDigit(r) && !(unicode.IsLetter(r) && unicode.IsLower(r)) && !unicode.Is(unicode.Lower, r) {
				return false
			}
			previousWasUnderscore = false
		}
	}

	return true
}

func toSnakeKebab(s string, separator rune) string {
	var (
		original = []rune(s)
		result   = make([]rune, 0, len(original))
	)
	for i, r := range original {

		if isSeparatorSymbol(r) { // Replace spaces, dots, and underscores with underscores
			if char, ok := lookupPrevChar(result, i); ok && char == r {
				continue // skip duplicates
			}
			if _, ok := lookupNextChar(original, i); !ok { // dispose "_" if it would be the last char
				continue
			}
			result = append(result, separator)
			continue
		}

		if unicode.IsUpper(r) { // Convert uppercase letters to lowercase and add underscore before them
			var prevOGChar, hasPrevOGChar = lookupPrevChar(original, i)
			if hasPrevOGChar && prevOGChar == separator {
				result = append(result, unicode.ToLower(r))
				continue
			}
			var (
				prevResChar, hasPrevResChar = lookupPrevChar(result, i)
				nextChar, hasNextChar       = lookupNextChar(original, i)
			)
			if hasPrevResChar && prevResChar != separator &&
				((hasPrevOGChar && unicode.IsLower(prevOGChar)) || (hasNextChar && unicode.IsLower(nextChar))) {
				result = append(result, separator)
			}
			result = append(result, unicode.ToLower(r))
			continue
		}

		// Add the current rune to the result string
		result = append(result, r)
	}
	return string(result)
}

func isSeparatorSymbol(r rune) bool {
	return r == '-' || r == ' ' || r == '.' || r == '_'
}

func lookupPrevChar(str []rune, index int) (rune, bool) {
	return lookupChar(str, index-1)
}

func lookupNextChar(str []rune, index int) (rune, bool) {
	return lookupChar(str, index+1)
}

func lookupChar(str []rune, index int) (rune, bool) {
	if !isIndexValid(str, index) {
		return 0, false
	}
	return str[index], true
}

func isIndexValid(str []rune, index int) bool {
	return 0 <= index && index < len(str)
}
