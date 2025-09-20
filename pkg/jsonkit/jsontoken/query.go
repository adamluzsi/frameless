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
	var in = toInput(r)
	return iterkit.Once2(func(yield func(json.RawMessage, error) bool) {
		{ // TODO: remove this, as it might be not in the intention of the user to close the io
			var callerNoLongerListens bool
			if closer, ok := in.(io.Closer); ok {
				defer func() {
					cErr := closer.Close()
					if !callerNoLongerListens {
						yield(nil, cErr)
					}
				}()
			}
		}
		const breakScanning errorkit.Error = "break"
		sc := Scanner{Selectors: []Selector{{
			Path: path,
			Func: func(raw json.RawMessage) error {
				if !yield(raw, nil) {
					return breakScanning
				}
				return nil
			},
		}}}
		_, err := sc.Scan(in)
		if errors.Is(err, breakScanning) {
			return
		}
		if err == nil {
			return
		}
		if !yield(nil, err) {
			return
		}
	})
}

func QueryMany(r io.Reader, selectors ...Selector) error {
	if len(selectors) == 0 {
		return nil
	}
	scanner := Scanner{Selectors: selectors}
	_, err := scanner.Scan(toInput(r))
	return err
}

func toInput(r io.Reader) Input {
	if i, ok := r.(Input); ok {
		return i
	} else {
		return bufio.NewReader(r)
	}
}
