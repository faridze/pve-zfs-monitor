package labrun

import (
	"errors"
	"strings"
)

// DeliveryMode selects notification handling for a lab run.
type DeliveryMode string

const (
	DeliveryNone DeliveryMode = "none"
	DeliveryFake DeliveryMode = "fake"
)

func ParseDeliveryMode(s string) (DeliveryMode, error) {
	switch DeliveryMode(strings.ToLower(strings.TrimSpace(s))) {
	case DeliveryNone:
		return DeliveryNone, nil
	case DeliveryFake:
		return DeliveryFake, nil
	default:
		return "", errors.New("delivery must be none or fake")
	}
}
