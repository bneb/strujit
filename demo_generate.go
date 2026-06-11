// Package main implements a data generator for the strujit benchmark.
// It emits a 1.6 GB memory block of randomized market tick data.
package main

import (
	"bufio"
	"encoding/binary"
	"log"
	"math"
	"math/rand"
	"os"
	"time"
)

const (
	// numTicks defines the total volume of generated trades (50M).
	numTicks = 50_000_000
	// bufSize optimizes write IO through a 16MB buffer.
	bufSize = 16 * 1024 * 1024
	// tickSize defines the total struct byte length with C-alignment.
	tickSize = 32
)

// main orchestrates the file generation pipeline.
func main() {
	f, err := os.Create("ticks.bin")
	if err != nil {
		log.Fatalf("failed to create ticks.bin: %v", err)
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, bufSize)
	buf := make([]byte, tickSize)

	// Seed cryptographic randomness to prevent benchmark caching.
	rand.Seed(time.Now().UnixNano())

	for i := 0; i < numTicks; i++ {
		generateTick(buf, i)
		if _, err := bw.Write(buf); err != nil {
			log.Fatalf("failed to write tick: %v", err)
		}
	}

	if err := bw.Flush(); err != nil {
		log.Fatalf("failed to flush buffer: %v", err)
	}
}

// generateTick injects randomized values into the pre-allocated struct buffer.
func generateTick(buf []byte, index int) {
	// symbol_id (offset 0:4)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(rand.Intn(100)))
	// padding 4:8 is intrinsically zeroed

	// timestamp_ns (offset 8:16)
	ts := uint64(time.Now().UnixNano() + int64(index))
	binary.LittleEndian.PutUint64(buf[8:16], ts)

	// price (offset 16:24)
	price := 100.0 + rand.Float64()*50.0
	binary.LittleEndian.PutUint64(buf[16:24], math.Float64bits(price))

	// size (offset 24:28)
	size := uint32(rand.Intn(10000))
	binary.LittleEndian.PutUint32(buf[24:28], size)
	// padding 28:32 is intrinsically zeroed
}
