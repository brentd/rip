# rip - Read In Parallel

`rip` is a small Go package for processing large files or streams in parallel on one machine. It was created to speed up the transformation of multi-GB files in varying formats inside of a single node ETL pipeline.

```
               process chunks of data on each CPU core

                                      >-e------------i----...->
                                      >-b----------h------...->
input: a--b--c--d--e--f--g... ->RIP-> >-a-------f---------...-> ... some output
                                      >-c-----------j-----...->
                                      >-d------------g----...->

```

This might be useful to you if:

  * You're processing large files, so you want to parse the data as a stream to transform or reduce it without loading it all into memory
  * Parsing/transforming/reducing on a single thread is CPU bound
  * Your data is mostly repeated records of the same type that can easily be broken into chunks and processed in any order - CSV is perfect, but even XML might be made to work (see examples)

It might be less useful if your files are terabytes large, in which case you're probably already using something distributed like Hadoop. This library is intended for the sweet spot where single node processing is faster, cheaper, and simpler than a distributed system, but naturally this depends on what you're doing.

You could also potentially use this library to turn a parser that can't normally operate on streams (e.g. some of Go's built-in decoders) into a streaming parser.

## Usage

Import the package:

```go
import (
	"github.com/brentd/rip"
)
```

```bash
go get "github.com/brentd/rip"
```

## CSV file example

```go
file, err := os.Open("huge.csv")
if err != nil {
  panic(err)
}
defer f.Close()

// Uses a default ChunkSize of 64 KiB and a ChunkBoundary of '\n'.
parallelReader := rip.NewParallelReader()

// Using Read() means that the ParallelReader will read as close as possible to
// a 64 KiB chunk each time without splitting a line between chunks. If you want
// a fixed chunk size that ignores ChunkBoundary, use ReadFixed().
parallelReader.Read(file, func(chunk []byte) {
  // This will be called from a pool of goroutines, where `chunk` is <= 64 KiB
  // of data terminated by and including '\n'.
})
```

## XML stdin example

```go
parallelReader := rip.NewParallelReader()

// defaults to runtime.NumCPU()
parallelReader.Concurrency = 10
// defaults to 64 KiB
parallelReader.ChunkSize = 1 << 20 // 1 MiB
// defaults to '\n'
parallelReader.ChunkBoundary = "</Product>"

parallelReader.Read(os.Stdin, func(chunk []byte) {
  // This will be called from a pool of goroutines, where `chunk` is <= 1 MiB of
  // data terminated by and including "</Product>". Depending on your data, this
  // means each chunk could contain thousands or a few full `<Product>` records,
  // but using `ChunkBoundary` ensures that data chunks never split a record.
})
```

## Memory Usage

Some notes about the library's internals that might be useful to understand:

  * To reduce allocations and work required by the GC, the reader thread reuses a fixed pool of byte buffers of size `ChunkSize`, one for each goroutine. In addition, one chunk per goroutine can be read ahead of the workers. That means your program's memory use will be at least `ChunkSize * Concurrency * 2` while a read is occurring. The default value of `Concurrency` is Go's `runtime.NumCPU()`. At the default chunk size of 64 KiB, a 6-core CPU would use `64 * 6 * 2 = 768 KiB`.
  * This also means that you should not try to use the raw byte array, `chunk`, outside of the callback provided to `Read()` without copying its data first (which you're probably already incidentally doing).

