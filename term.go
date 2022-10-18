package main

import (
	"time"

	"golang.org/x/term"
	"k8s.io/client-go/tools/remotecommand"
)

type dimQueue chan remotecommand.TerminalSize

// monitor spawns a goroutine to poll the current terminal dimensions and
// enqueues them into a dimQueue. A chan is returned that can be used to signal
// the shutdown of this goroutine.
//
// TODO: This should use SIGWINCH and not poll the terminal dimensions
func (q dimQueue) monitor() chan bool {
	cancel := make(chan bool)

	go func() {
		for {
			select {
			case <-cancel:
				close(cancel)
				return
			case <-time.After(5 * time.Second):
				q.update()
			}
		}
	}()

	return cancel
}

func (q dimQueue) Next() *remotecommand.TerminalSize {
	newDim, ok := <-q
	if !ok {
		return nil
	}

	return &newDim
}

func (q dimQueue) update() {
	width, height, err := term.GetSize(0)
	if err != nil {
		panic(err)
	}
	q <- remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)}
}
