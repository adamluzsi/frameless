package specs

import (
	"fmt"
	"os"
	"strconv"
)

var benchmarkSamplingCount int

func init() {
	benchmarkSamplingCount = 128

	bsc, ok := os.LookupEnv(`BENCHMARK_SAMPLING_COUNT`)
	if !ok {
		return
	}

	i, err := strconv.Atoi(bsc)
	if err != nil {
		fmt.Println(fmt.Sprintf(`WARNING - BENCHMARK_SAMPLING_COUNT env var value not convertable to int, will be ignored`))
		return
	}

	benchmarkSamplingCount = i
}

func BenchmarkSamplingCount() int {
	return benchmarkSamplingCount
}
