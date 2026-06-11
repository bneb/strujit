package main

import (
	"encoding/binary"
	"math"
	"os"
)

func main() {
	f, err := os.Create("data.bin")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	buf := make([]byte, 24)
	for i := uint32(0); i < 100000; i++ {
		// Id
		binary.LittleEndian.PutUint32(buf[0:4], i)
		// Padding 4:7 is zero
		// Price
		price := float64(i) * 1.5
		binary.LittleEndian.PutUint64(buf[8:16], math.Float64bits(price))
		// Side
		buf[16] = uint8(i % 2)
		// Padding 17:23 is zero
		f.Write(buf)
	}
}
