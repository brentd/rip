package rip

import (
	"bufio"
	"bytes"
	"io"
	"runtime"
	"sync"
)

type ParallelReader struct {
	Concurrency   int
	ChunkSize     int
	ChunkBoundary string
	chunks        chan *chunk
	pool          *Pool
}

func NewParallelReader() *ParallelReader {
	r := new(ParallelReader)
	r.Concurrency = runtime.NumCPU()
	r.ChunkBoundary = "\n"
	r.ChunkSize = 1 << 16 // 64 KiB
	r.chunks = make(chan *chunk, r.Concurrency)

	return r
}

// Read takes an input io.Reader stream and calls the passed callback from a
// pool of goroutines, once per chunk. Your callback could receive chunks in any
// order.
func (r *ParallelReader) Read(stream io.Reader, work func(chunk []byte)) {
	r.pool = NewPool(r.Concurrency, r.ChunkSize)

	scanner := bufio.NewScanner(stream)

	scanBuf := make([]byte, r.ChunkSize)
	scanner.Buffer(scanBuf, r.ChunkSize)

	// Start the worker goroutines that receive chunks of data in parallel.
	wg := r.startWorkers(work)

	// Scan the input stream in the foreground, splitting data into chunks as
	// close to ChunkSize as possible while respecting ChunkBoundary.
	scanner.Split(r.ScanChunksWithBoundary)
	for scanner.Scan() {
		// Scanner reuses its internal buffer while scanning, so in order to safely
		// pass the bytes to a channel where they will be read concurrently, we have
		// to copy them. Rather than allocating a new block of memory each time, we
		// reuse an existing pool of buffers.
		token := scanner.Bytes()
		buf := r.pool.Borrow()

		if len(token) > 0 {
			size := copy(buf, token)
			r.chunks <- &chunk{buffer: buf, readableSize: size}
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	close(r.chunks)
	wg.Wait()
}

// ReadFixed is a specialized, faster implementation when the input stream can
// be split into fixed size chunks without needing to respect a record boundary.
// The final chunk will be less than ChunkSize if the stream or file's length is
// not evenly divisible by ChunkSize.
func (r *ParallelReader) ReadFixed(stream io.Reader, work func(chunk []byte)) {
	r.pool = NewPool(r.Concurrency, r.ChunkSize)

	wg := r.startWorkers(work)

	for {
		buf := r.pool.Borrow()

		// io.ReadFull() will read up to cap(buf) if it doesn't reach EOF first. If it
		// does encounter an EOF before buf is full, the actual read size is
		// returned and err will be io.ErrUnexpectedEOF.
		actualReadSize, err := io.ReadFull(stream, buf)
		chunk := chunk{buffer: buf, readableSize: actualReadSize}

		if err == nil {
			r.chunks <- &chunk
			continue
		}
		if err != nil {
			// We're at EOF, but there's still some data, so send it to the channel
			// before finishing.
			if err == io.ErrUnexpectedEOF {
				r.chunks <- &chunk
				close(r.chunks)
				break
				// We arrived at EOF with nothing left to read. We're done!
			} else if err == io.EOF {
				close(r.chunks)
				break
			} else {
				panic(err)
			}
		}
	}

	wg.Wait()
}

func (r *ParallelReader) startWorkers(fn func(chunk []byte)) *sync.WaitGroup {
	var wg sync.WaitGroup
	wg.Add(r.Concurrency)
	for i := 0; i < r.Concurrency; i++ {
		go func() {
			defer wg.Done()
			for chunk := range r.chunks {
				fn(chunk.ReadableBytes())
				r.pool.Return(chunk.buffer)
			}
		}()
	}
	return &wg
}

// Custom bufio.Scanner split function that returns chunks of bytes as close to
// the configured ChunkSize as possible, while respecting the record boundary
// specified by ChunkBoundary. See bufio.Scanner documentation for more details
// about this method.
func (r *ParallelReader) ScanChunksWithBoundary(data []byte, atEOF bool) (advance int, token []byte, err error) {
	// Request more data until we've read up to at least our desired chunk size.
	if !atEOF && len(data) < r.ChunkSize {
		return 0, nil, nil
	}

	// Now that we have the desired chunk size, return the slice of the buffer
	// that ends with ChunkBoundary, instructing the Scanner to advance to the end
	// of the boundary on the next read.
	idx := bytes.LastIndex(data, []byte(r.ChunkBoundary))
	if idx > -1 {
		boundaryEnd := idx + len(r.ChunkBoundary)
		return boundaryEnd, data[:boundaryEnd], nil
	}

	// If we weren't able to find a boundary, but we're not yet at EOF, request
	// more data. bufio.Scanner.Scan() will return false and set Err() if we reach
	// the maximum buffer length but still haven't been able to find a chunk.
	if !atEOF {
		return 0, nil, nil
	}

	// There is one final token to be delivered, which may be an empty string.
	// Returning bufio.ErrFinalToken here tells Scan there are no more tokens
	// after this but does not trigger an error to be returned from Scan itself.
	return 0, data, bufio.ErrFinalToken
}

// Stores the backing buffer and length at which a receiver will need to slice
// the backing buffer to get a full "token".
type chunk struct {
	readableSize int
	buffer       []byte
}

func (chunk *chunk) ReadableBytes() []byte {
	return chunk.buffer[:chunk.readableSize]
}

type Pool struct {
	pool       chan []byte
	bufferSize int
}

func NewPool(max int, bufferSize int) *Pool {
	return &Pool{
		pool:       make(chan []byte, max),
		bufferSize: bufferSize,
	}
}

func (p *Pool) Borrow() []byte {
	var c []byte
	// select will go to the default case if receiving from the channel would
	// block (i.e. it's empty)
	select {
	case c = <-p.pool:
	default:
		// If no buffer is available, make a new one
		c = make([]byte, p.bufferSize)
	}
	return c
}

func (p *Pool) Return(c []byte) {
	// select will go to the default case if sending to the channel would block
	// (i.e. it's full)
	select {
	case p.pool <- c:
	default:
		// If the pool (channel) is full, no-op; let the object get GC'd
	}
}
