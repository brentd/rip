# rip - Read In Parallel

`rip` provides a simple interface for processing large files or streams in parallel. It was created to speed up the transformation of multi-GB files in varying formats into a common format inside of an ETL pipeline.

```
          process chunks of data on each CPU core

                                      >-e------------i----...->
                                      >-b----------h------...->
input: a--b--c--d--e--f--g... ->RIP-> >-a-------f---------...-> ... some output
                                      >-c-----------j-----...->
                                      >-d------------g----...->

```

This might be useful to you if:

  * You're processing large files, so you want to parse it as a stream to reduce or transform the data without loading it all into memory
  * Parsing/transforming/reducing is CPU bound *or* you want to use a parser that can only parse in-memory data in a streaming fashion
  * Your data is mostly repeated records of the same type that can be broken into chunks and processed in any order

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