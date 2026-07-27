package worker

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	errFrameTooLarge  = errors.New("NDJSON frame exceeds size limit")
	errTruncatedFrame = errors.New("NDJSON frame ended before newline")
)

func readFrame(
	reader *bufio.Reader,
	maxPayloadBytes int,
) ([]byte, error) {
	payload := make([]byte, 0, min(maxPayloadBytes, readBufferBytes))
	for {
		fragment, err := reader.ReadSlice('\n')
		hasNewline := len(fragment) != 0 && fragment[len(fragment)-1] == '\n'
		if hasNewline {
			fragment = fragment[:len(fragment)-1]
		}
		nextLength := len(payload) + len(fragment)
		possibleCRLFTerminator := nextLength == maxPayloadBytes+1 &&
			((len(fragment) != 0 && fragment[len(fragment)-1] == '\r') ||
				(len(fragment) == 0 &&
					len(payload) != 0 &&
					payload[len(payload)-1] == '\r'))
		if nextLength > maxPayloadBytes && !possibleCRLFTerminator {
			return nil, fmt.Errorf(
				"%w: maximum is %d bytes",
				errFrameTooLarge,
				maxPayloadBytes,
			)
		}
		payload = append(payload, fragment...)

		if hasNewline {
			if len(payload) != 0 && payload[len(payload)-1] == '\r' {
				payload = payload[:len(payload)-1]
			}
			if len(payload) > maxPayloadBytes {
				return nil, fmt.Errorf(
					"%w: maximum is %d bytes",
					errFrameTooLarge,
					maxPayloadBytes,
				)
			}
			return payload, nil
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(payload) != 0:
			return nil, errTruncatedFrame
		case err != nil:
			return nil, err
		default:
			return nil, errTruncatedFrame
		}
	}
}

type boundedTail struct {
	mu       sync.Mutex
	bytes    []byte
	capacity int
}

func newBoundedTail(capacity int) *boundedTail {
	return &boundedTail{
		bytes:    make([]byte, 0, capacity),
		capacity: capacity,
	}
}

func (tail *boundedTail) Write(data []byte) (int, error) {
	tail.mu.Lock()
	defer tail.mu.Unlock()

	originalLength := len(data)
	if len(data) >= tail.capacity {
		tail.bytes = append(tail.bytes[:0], data[len(data)-tail.capacity:]...)
		return originalLength, nil
	}

	overflow := len(tail.bytes) + len(data) - tail.capacity
	if overflow > 0 {
		copy(tail.bytes, tail.bytes[overflow:])
		tail.bytes = tail.bytes[:len(tail.bytes)-overflow]
	}
	tail.bytes = append(tail.bytes, data...)
	return originalLength, nil
}

func (tail *boundedTail) String() string {
	tail.mu.Lock()
	defer tail.mu.Unlock()
	return string(tail.bytes)
}
