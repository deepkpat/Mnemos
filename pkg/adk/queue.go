package adk

import (
	"errors"
	"sync"
)

// Queue represents a thread-safe FIFO data structure
type Queue[T any] struct {
	items []T
	lock  sync.Mutex
}

// Enqueue adds an element to the back of the queue
func (q *Queue[T]) Enqueue(item T) {
	q.lock.Lock()
	defer q.lock.Unlock()
	q.items = append(q.items, item)
}

// Dequeue removes and returns the element at the front of the queue
func (q *Queue[T]) Dequeue() (T, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if len(q.items) == 0 {
		var zero T
		return zero, errors.New("queue is empty")
	}

	item := q.items[0]
	q.items = q.items[1:]
	return item, nil
}

// Drain removes and returns all the elements in the queue
func (q *Queue[T]) Drain() []T {
	q.lock.Lock()
	defer q.lock.Unlock()

	items := q.items
	q.items = []T{}
	return items
}

// Peek returns the front element without removing it
func (q *Queue[T]) Peek() (T, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if len(q.items) == 0 {
		var zero T
		return zero, errors.New("queue is empty")
	}

	return q.items[0], nil
}

// IsEmpty returns true if the queue has no elements
func (q *Queue[T]) IsEmpty() bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	return len(q.items) == 0
}

// Size returns the number of elements in the queue
func (q *Queue[T]) Size() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	return len(q.items)
}
