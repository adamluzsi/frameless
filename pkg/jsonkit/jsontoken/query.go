package jsontoken

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"iter"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase/pp"
)

// Query will turn the input reader into a json visitor that yields results when a path is matching.
// Think about it something similar as jq.
// It will not keep the visited json i n memory, to avoid problems with infinite streams.
func Query(r io.Reader, path ...Kind) iter.Seq2[json.RawMessage, error] {
	const stopIteration errorkit.Error = "break"
	return func(yield func(json.RawMessage, error) bool) {

		var stop bool
		sc := Scanner{Selectors: []Selector{{
			Path: path,
			On: func(src io.Reader) error {
				var ok bool
				defer func() {
					if !ok {
						pp.PP("!ok")
						stop = true
					}
				}()
				pp.PP("yield:before")
				cont := yield(io.ReadAll(src))
				pp.PP("yield:after")
				ok = true
				if !cont {
					stop = true
					return stopIteration
				}
				return nil
			},
		}}}
		pp.PP("res", "stop", stop)
		var err = sc.Scan(toInput(r))
		pp.PP(err)
		if errors.Is(err, stopIteration) {
			return
		}
		if stop {
			return
		}
		if err != nil {
			yield(nil, err)
		}
	}
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
