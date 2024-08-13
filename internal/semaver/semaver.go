package semaver

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
)

var rgxSemanticVersion = regexp.MustCompile(`^(v|V)?(\d+)(?:\.(\d+))?(?:\.(\d+)(?:-(\d+(?:[-.]\w+)*)?)?(?:\+(\w+(?:[-.]\w+)*))?)?$`)

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

	switch cmpPreTag(v.Pre, oth.Pre) {
	case -1:
		return true
	// case 0: // if pre tags are equal we continue the comparison to
	case 1:
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

func cmpPreTag(t1, t2 string) int {
	t1 = strings.ToLower(t1)
	t2 = strings.ToLower(t2)
	p1 := preTagPriority(t1)
	p2 := preTagPriority(t2)
	if p1 < p2 {
		return -1
	}
	if p2 < p1 {
		return 1
	}
	if p1 == p2 {
		if t1 == t2 {
			return 0
		}
		if t1 < t2 {
			return -1
		}
		if t2 < t1 {
			return 1
		}
	}
	panic("not implemented") // should not happen
}

func preTagPriority(tag string) int {
	if tag == "" { // If pre-release is present, it's considered lower priority than a version without one
		return -1
	}
	tag = strings.ToLower(tag)
	for i, cmpTag := range preTags {
		if strings.HasPrefix(tag, cmpTag) {
			return i
		}
	}
	return math.MaxInt
}

func MustParseVersion(v string) Version {
	version, err := Parse(v)
	if err != nil {
		panic(err)
	}
	return version
}

const ErrParse errorkit.Error = "ErrParse"

func Parse(str string) (_ Version, returnErr error) {
	defer func() {
		if returnErr != nil {
			returnErr = errorkit.Merge(ErrParse, returnErr)
		}
	}()

	match := rgxSemanticVersion.FindStringSubmatch(string(str))

	if len(match) == 0 {
		return Version{}, fmt.Errorf("unable to parse version out from %q", str)
	}

	var parseVersionNum = func(raw string, name string) (int, error) {
		if raw == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("failed to parse %s version: %q -> %w", name, raw, err)
		}
		return n, nil
	}

	var (
		v   Version
		err error
	)

	if 2 < len(match) {
		v.Major, err = parseVersionNum(match[2], "major")
		if err != nil {
			return v, err
		}
	}

	if 3 < len(match) {
		v.Minor, err = parseVersionNum(match[3], "minor")
		if err != nil {
			return v, err
		}
	}

	if 4 < len(match) {
		v.Patch, err = parseVersionNum(match[4], "patch")
		if err != nil {
			return v, err
		}
	}

	if 5 < len(match) {
		v.Pre = match[5]
	}

	if 6 < len(match) {
		v.Build = match[6]
	}

	return v, nil
}
