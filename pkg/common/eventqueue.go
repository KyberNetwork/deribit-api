package common

import (
	"errors"
)

const (
	maxQueueLength = 100
)

type EventQueue struct {
	q []interface{} // queue
	s int           // start index
}

func NewEventQueue() *EventQueue {
	return &EventQueue{
		q: make([]interface{}, maxQueueLength),
	}
}

func (eq *EventQueue) Insert(e interface{}, index int) error {
	if index < 0 || index >= maxQueueLength {
		return errors.New("index out of range")
	}

	eq.q[index] = e
	return nil
}

func (eq *EventQueue) GetOffset() int {
	return eq.s
}

func (eq *EventQueue) GetEvent() interface{} {
	return eq.q[eq.s]
}

func (eq *EventQueue) Next() {
	eq.q[eq.s] = nil
	eq.s = (eq.s + 1) % maxQueueLength
}
