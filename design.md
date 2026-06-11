# HFT Market Data Benchmark Design

## 1. Schema & Data Generation
**Schema:** `uint32 symbol, uint64 ts, float64 price, uint32 size`
**Total Size:** 32 bytes (with C-struct alignment: 4 byte pad after symbol, 4 byte pad after size).
**Volume:** 50,000,000 structs = 1.6 GB.
**Randomness:** Use `math/rand` initialized with current time. `size` uniformly random between `0` and `10000`. We target trades `size > 9900` (approx 1% = 500k structs = 16 MB).

## 2. strujit modification
Add `uint64` (size 8, align 8, `i64`) to `strujit` `main.go` parser.

## 3. The Go Benchmark
We will write the most optimized pure Go implementation (`demo_benchmark_go.go`) using `syscall.Mmap` and `binary.LittleEndian` to avoid `io.Reader` overhead. This ensures we are beating the *best* Go has to offer, not a naive `encoding/binary.Read` implementation. We will write matching structs to `/dev/null` or a file to mirror `strujit`'s I/O pattern.

## 4. Execution & Validation
Run `time strujit ... > blocks_jit.bin`
Run `time ./demo_benchmark_go > blocks_go.bin`
Verify hashes of both outputs to prove identical behavior and no faked results.
