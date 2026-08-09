package updater

import (
	"errors"
	"strconv"
	"strings"
)

type semanticVersion struct {
	major uint64
	minor uint64
	patch uint64
	pre   []string
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	if value == "" || strings.HasPrefix(value, "v") || strings.TrimSpace(value) != value {
		return semanticVersion{}, errors.New("version must be strict SemVer without a v prefix")
	}
	coreAndPre := value
	if plus := strings.IndexByte(value, '+'); plus >= 0 {
		if err := validateIdentifiers(value[plus+1:], false); err != nil {
			return semanticVersion{}, errors.New("invalid SemVer build metadata")
		}
		coreAndPre = value[:plus]
	}
	var pre []string
	core := coreAndPre
	if dash := strings.IndexByte(coreAndPre, '-'); dash >= 0 {
		preValue := coreAndPre[dash+1:]
		if err := validateIdentifiers(preValue, true); err != nil {
			return semanticVersion{}, errors.New("invalid SemVer prerelease")
		}
		pre = strings.Split(preValue, ".")
		core = coreAndPre[:dash]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, errors.New("version must contain major.minor.patch")
	}
	numbers := make([]uint64, 3)
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, errors.New("SemVer numeric identifiers cannot have leading zeros")
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return semanticVersion{}, errors.New("SemVer core identifiers must be numeric")
			}
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, errors.New("SemVer numeric identifier is too large")
		}
		numbers[i] = n
	}
	return semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2], pre: pre}, nil
}

func validateIdentifiers(value string, numericLeadingZero bool) error {
	if value == "" {
		return errors.New("empty identifiers")
	}
	for _, part := range strings.Split(value, ".") {
		if part == "" {
			return errors.New("empty identifier")
		}
		numeric := true
		for _, r := range part {
			if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '-') {
				return errors.New("invalid identifier")
			}
			if r < '0' || r > '9' {
				numeric = false
			}
		}
		if numericLeadingZero && numeric && len(part) > 1 && part[0] == '0' {
			return errors.New("numeric prerelease identifier has a leading zero")
		}
	}
	return nil
}

func compareSemanticVersions(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.pre) == 0 && len(right.pre) == 0 {
		return 0
	}
	if len(left.pre) == 0 {
		return 1
	}
	if len(right.pre) == 0 {
		return -1
	}
	limit := min(len(left.pre), len(right.pre))
	for i := 0; i < limit; i++ {
		if result := comparePrereleaseIdentifier(left.pre[i], right.pre[i]); result != 0 {
			return result
		}
	}
	if len(left.pre) < len(right.pre) {
		return -1
	}
	if len(left.pre) > len(right.pre) {
		return 1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftErr := strconv.ParseUint(left, 10, 64)
	rightNumber, rightErr := strconv.ParseUint(right, 10, 64)
	switch {
	case leftErr == nil && rightErr == nil:
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	case leftErr == nil:
		return -1
	case rightErr == nil:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
