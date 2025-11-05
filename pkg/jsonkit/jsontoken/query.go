package jsontoken

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"iter"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/synckit"
)

// Query will turn the input reader into a json visitor that yields results when a path is matching.
// Think about it something similar as jq.
// It will not keep the visited json i n memory, to avoid problems with infinite streams.
func Query(r io.Reader, path ...Kind) iter.Seq2[json.RawMessage, error] {
	const stopIteration errorkit.Error = "break"
	type Hit struct {
		Data json.RawMessage
		Err  error
	}
	return func(yield func(json.RawMessage, error) bool) {
		var hits = make(chan Hit)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sc := Scanner{
			Selectors: []Selector{{
				Path: path,
				On: func(src io.Reader) error {
					data, err := io.ReadAll(src)
					select {
					case hits <- Hit{Data: data, Err: err}:
					case <-ctx.Done():
						return stopIteration
					}
					return nil
				},
			}},
		}

		var g synckit.Group
		defer g.Wait()

		g.Go(func(ctx context.Context) error {
			defer close(hits)
			return sc.Scan(toInput(r))
		})

		for hit := range hits {
			if !yield(hit.Data, hit.Err) {
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
