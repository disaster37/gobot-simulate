package state

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidID is returned when an identifier does not match the simulator's
// restricted identifier alphabet.
var ErrInvalidID = errors.New("simulate: invalid identifier")

var identifierPattern = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,64}$`)

// ValidateID validates a board, device, pin, value, or function identifier.
func ValidateID(id string) error {
	if identifierPattern.MatchString(id) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrInvalidID, id)
}

// ValidateIdentifier validates id and includes the identifier kind in errors.
func ValidateIdentifier(kind, id string) error {
	if err := ValidateID(id); err != nil {
		return fmt.Errorf("%w: %s %q", ErrInvalidID, kind, id)
	}
	return nil
}

// NormalizeIdentifier converts an arbitrary label to a valid identifier. It
// is only used for generated defaults, never to accept untrusted input.
func NormalizeIdentifier(id string) string {
	if id == "" {
		return ""
	}
	out := make([]byte, 0, len(id))
	for i := 0; i < len(id) && len(out) < 64; i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == ':', c == '-':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "device"
	}
	return string(out)
}
