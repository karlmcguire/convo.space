package main

import (
	"errors"
	"fmt"
	"net/http"
)

// User is the struct for each connected client.
type User struct {
	// Pipe is the raw data channel for sending data to the user
	Pipe chan []byte
	// Stop is the channel for stopping the Listen() goroutine
	Stop chan struct{}
	// IP is the user's IP address
	IP string
	// UserId is user's id in the parent conversation
	UserId int
	// ConvoId is the convoId of the parent conversation
	ConvoId string
	// Writer is the open http.ResponseWriter
	Writer http.ResponseWriter
	// Request is the initial request
	Request *http.Request
}

// NewUser creates a NewUser object with the needed http variables.
func NewUser(w http.ResponseWriter, r *http.Request) *User {
	return &User{
		Pipe:    make(chan []byte),
		IP:      GetIP(r.RemoteAddr),
		Writer:  w,
		Request: r,
	}
}

// Listen is a goroutine running for as long as the client stays connected. It
// uses SSE to send events (messages/notifications) over HTTPS.
func (u *User) Listen() error {
	var (
		// flusher is for establishing a SSE connection
		flusher http.Flusher
		// notify waits for the user to close the connection
		notify <-chan bool

		ok bool
	)

	// try to establish a SSE connection
	if flusher, ok = u.Writer.(http.Flusher); !ok {
		return errors.New("couldn't get flusher")
	}

	// set the headers
	u.Writer.Header().Set("Content-Type", "text/event-stream")
	u.Writer.Header().Set("Cache-Control", "no-cache")
	u.Writer.Header().Set("Connection", "keep-alive")
	u.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// create the close notifier to determine when the client closes
	notify = u.Writer.(http.CloseNotifier).CloseNotify()
	// this goroutine waits for the user to close the connection, and does
	// the needed cleanup
	go func() {
		// wait for the user to close the connection
		<-notify
		// delete the user from the global Store variable
		Store.DeleteUser(u.ConvoId, u.UserId)
		// stop the for loop in the parent function
		u.Stop <- struct{}{}

		close(u.Pipe)
	}()

	for {
		select {
		// new data is coming in (notification/message)
		case data := <-u.Pipe:
			// write the data
			fmt.Fprintf(u.Writer, "%s\n", data)
			flusher.Flush()
		// time to stop
		case <-u.Stop:
			return nil
		}
	}
}

// Write is a helper function for writing to the user's channel.
func (u *User) Write(data []byte) {
	u.Pipe <- data
}
