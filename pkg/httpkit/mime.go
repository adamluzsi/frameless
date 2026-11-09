package httpkit

import (
	"cmp"
	"mime"
	"slices"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/httpkit/mediatype"
)

func lookupMediaType(mimeType string) (mediatype.MediaType, bool) {
	if mimeType == "" {
		return mimeType, false
	}
	mt, _, err := mime.ParseMediaType(mimeType)
	if err != nil || mt == "" {
		if mimeType != "" {
			return mimeType, true
		}
		return mt, false
	}
	return mt, true
}

// parseAccept parses the value of an Accept header (RFC 9110 §12.5.1).
// It returns the acceptable media ranges, ordered by their preference,
// and the media types excluded with a q=0 weight.
func parseAccept(accept string) (ranges []string, excluded map[string]struct{}) {
	type acceptable struct {
		MediaRange string
		Weight     float64
	}
	var acceptables []acceptable
	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mediaRange, params, err := mime.ParseMediaType(part)
		if err != nil {
			// tolerate the malformed parameters of a media range
			mediaRange, _, _ = strings.Cut(part, ";")
			mediaRange = strings.ToLower(strings.TrimSpace(mediaRange))
			params = nil
		}
		if mediaRange == "" {
			continue
		}
		var weight = 1.0
		if q, ok := params["q"]; ok {
			if w, err := strconv.ParseFloat(q, 64); err == nil {
				weight = w
			}
		}
		if weight <= 0 {
			if excluded == nil {
				excluded = make(map[string]struct{})
			}
			excluded[mediaRange] = struct{}{}
			continue
		}
		acceptables = append(acceptables, acceptable{MediaRange: mediaRange, Weight: weight})
	}
	slices.SortStableFunc(acceptables, func(a, b acceptable) int {
		return cmp.Compare(b.Weight, a.Weight)
	})
	for _, a := range acceptables {
		ranges = append(ranges, a.MediaRange)
	}
	return ranges, excluded
}

// mediaRangeMatches reports whether a media type is in a media range, like "*/*" or "application/*".
func mediaRangeMatches(mediaRange string, mediaType mediatype.MediaType) bool {
	if mediaRange == "*/*" {
		return true
	}
	rangeType, rangeSubtype, ok := strings.Cut(mediaRange, "/")
	if !ok {
		return false
	}
	typ, subtype, ok := strings.Cut(mediaType, "/")
	if !ok {
		return false
	}
	if !strings.EqualFold(rangeType, typ) {
		return false
	}
	return rangeSubtype == "*" || strings.EqualFold(rangeSubtype, subtype)
}
