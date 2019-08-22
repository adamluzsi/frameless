package specs

import (
	"fmt"
	"os"
	"strconv"
)

const msgNotMeasurable = `not measurable spec`

var benchmarkEntityVolumeCount int

func init() {
	benchmarkEntityVolumeCount = 128

	bsc, ok := os.LookupEnv(`BENCHMARK_ENTITY_VOLUME_COUNT`)
	if !ok {
		return
	}

	i, err := strconv.Atoi(bsc)
	if err != nil {
		fmt.Println(fmt.Sprintf(`WARNING - BENCHMARK_ENTITY_VOLUME_COUNT env var value not convertable to int, will be ignored`))
		return
	}

	benchmarkEntityVolumeCount = i
}

func BenchmarkEntityVolumeCount() int {
	return benchmarkEntityVolumeCount
}
