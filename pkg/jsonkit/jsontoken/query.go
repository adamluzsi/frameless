package jsontoken

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/synckit"
)

// Query will turn the input reader into a json visitor that yields results when a path is matching.
// Think about it something similar as jq.
// It will not keep the visited json i n memory, to avoid problems with infinite streams.
func Query(r io.Reader, path ...Kind) iter.Seq2[json.RawMessage, error] {
	const stopIteration errorkitlite.Error = "break"
	return func(yield func(json.RawMessage, error) bool) {
		var dch = make(chan []byte)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sc := Scanner{
			Selectors: []Selector{{
				Path: path,
				Func: func(data []byte) error {
					select {
					case dch <- data:
					case <-ctx.Done():
						return stopIteration
					}
					return nil
				},
			}},
		}

		var g synckit.Group
		defer g.Wait()

		g.Go(nil, func(ctx context.Context) error {
			defer close(dch)
			return sc.Scan(toInput(r))
		})

		for data := range dch {
			if !yield(data, nil) {
				return
			}
		}

		var err = g.Wait()
		if errors.Is(err, stopIteration) {
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
