package memory

import "sync"

type RingBuffer[T any] struct {
	data  []T
	head  int
	tail  int
	size  int
	cap   int
	mutex sync.RWMutex
}

func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		data: make([]T, capacity),
		cap:  capacity,
	}
}

func (rb *RingBuffer[T]) Push(item T) (evicted T, hasEvicted bool) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	if rb.size == rb.cap {
		evicted = rb.data[rb.head]
		hasEvicted = true
		rb.head = (rb.head + 1) % rb.cap
		rb.size--
	}

	rb.data[rb.tail] = item
	rb.tail = (rb.tail + 1) % rb.cap
	rb.size++

	return evicted, hasEvicted
}

func (rb *RingBuffer[T]) GetAll() []T {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	result := make([]T, rb.size)
	for i := 0; i < rb.size; i++ {
		idx := (rb.head + i) % rb.cap
		result[i] = rb.data[idx]
	}
	return result
}

func (rb *RingBuffer[T]) GetRecent(n int) []T {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()

	if n > rb.size {
		n = rb.size
	}

	result := make([]T, n)
	for i := 0; i < n; i++ {
		idx := (rb.tail - n + i + rb.cap) % rb.cap
		result[i] = rb.data[idx]
	}
	return result
}

func (rb *RingBuffer[T]) Size() int {
	rb.mutex.RLock()
	defer rb.mutex.RUnlock()
	return rb.size
}

func (rb *RingBuffer[T]) Capacity() int {
	return rb.cap
}

func (rb *RingBuffer[T]) Clear() {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	rb.head = 0
	rb.tail = 0
	rb.size = 0
	rb.data = make([]T, rb.cap)
}
