package semaver

import (
	"fmt"
	"regexp"
	"strconv"
)

// Semantic Versioning //

var rgxSemanticVersion = regexp.MustCompile(`^(v|V)?(\d+)(?:\.(\d+)(?:\.(\d+)(?:-(\d+(?:[-.]\w+)*)?)?(?:\+(\w+(?:[-.]\w+)*))?))$`)

type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string
	Build string
}

func (v Version) Less(oth Version) bool {
	if v.Major < oth.Major {
		return true
	}
	if v.Major > oth.Major {
		return false
	}

	if v.Minor < oth.Minor {
		return true
	}
	if v.Minor > oth.Minor {
		return false
	}

	if v.Patch < oth.Patch {
		return true
	}
	if v.Patch > oth.Patch {
		return false
	}

	// If pre-release is present, it's considered less than a version without one
	if v.Pre != "" && oth.Pre == "" {
		return true
	}
	if v.Pre == "" && oth.Pre != "" {
		return false
	}

	// Compare pre-releases lexicographically
	if v.Pre < oth.Pre {
		return true
	}
	if v.Pre > oth.Pre {
		return false
	}

	// build data just compared as
	if v.Build < oth.Build {
		return true
	}

	return false
}

var preTags = []string{
	"alpha",
	"beta",
	"gamma",
	"delta",
	"epsilon",
	"zeta",
	"rc",
	"releasecandidate",
	"prerelease",
	"snapshot",
	"nightly",
}

func MustParseVersion(v string) Version {
	version, err := ParseVersion(v)
	if err != nil {
		panic(err)
	}
	return version
}

func ParseVersion(str string) (Version, error) {
	match := rgxSemanticVersion.FindStringSubmatch(string(str))

	if len(match) == 0 {
		return Version{}, fmt.Errorf("unable to parse version out from %q", str)
	}

	var v Version

	if 2 < len(match) {
		raw := match[2]
		major, err := strconv.Atoi(raw)
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse major version: %q -> %w", raw, err)
		}
		v.Major = major
	} else {
		v.Major = 0
	}

	if 3 < len(match) {
		raw := match[3]
		minor, err := strconv.Atoi(raw)
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse minor version: %q -> %w", raw, err)
		}
		v.Minor = minor
	} else {
		v.Minor = 0
	}

	if 4 < len(match) {
		raw := match[4]
		patch, err := strconv.Atoi(raw)
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse patch version: %q -> %w", raw, err)
		}
		v.Patch = patch
	} else {
		v.Patch = 0
	}

	if 5 < len(match) {
		v.Pre = match[5]
	}

	if 6 < len(match) {
		v.Build = match[6]
	}

	return v, nil
}
