package jsontoken

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"iter"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
)

// Query will turn the input reader into a json visitor that yields results when a path is matching.
// Think about it something similar as jq.
// It will not keep the visited json i n memory, to avoid problems with infinite streams.
func Query(r io.Reader, path ...Kind) iter.Seq2[json.RawMessage, error] {
	const stopIteration errorkit.Error = "break"
	return iterkit.Once2(func(yield func(json.RawMessage, error) bool) {
		sc := Scanner{Selectors: []Selector{{
			Path: path,
			Func: func(raw json.RawMessage) error {
				if !yield(raw, nil) {
					return stopIteration
				}
				return nil
			},
		}}}
		err := sc.Scan(toInput(r))
		if errors.Is(err, stopIteration) {
			return
		}
		if err != nil {
			yield(nil, err)
		}
	})
}

func QueryMany(r io.Reader, selectors ...Selector) error {
	if len(selectors) == 0 {
		return nil
	}
	scanner := Scanner{Selectors: selectors}
	return scanner.Scan(toInput(r))
}

func toInput(r io.Reader) Input {
	if i, ok := r.(Input); ok {
		return i
	} else {
		return bufio.NewReader(r)
	}
}
