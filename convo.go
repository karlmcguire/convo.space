package main

import (
	"errors"
	"time"
)

// Convo is the container for a conversation.
type Convo struct {
	// ConvoId is the unique conversation id needed to access the conversation
	ConvoId string
	// Users is the array containing both parties of the conversation, some
	// may be nil
	Users [2]*User
	// Messages contains unread messages of the conversation, where the
	// messageId is the key and the value is the raw data of the message
	Messages map[string][]byte
	// Stop is just to notify the pinging goroutine to stop (when the
	// conversation is deleted)
	Stop chan struct{}
}

// Ping is a goroutine that continuously pings each user in the conversation.
//
// TODO: This function serves to make sure the client's connection isn't closed
//		 but there are probably better ways to do that. Check net/http settings
//		 to see if I can change the timeout settings for the web server.
func (c *Convo) Ping() {
	for {
		select {
		// end the goroutine
		case <-c.Stop:
			return
		// ping every 30 seconds
		case <-time.After(time.Second * 30):
			c.Broadcast([]byte("."))
		}
	}
}

// CreateMessage creates a new message from raw data and adds it to the
// conversation. It returns the new messageId, and might return an error.
// There might be an error from a problem generating the new messageId, or a
// messageId collision.
//
// TODO: Figure out how to handle messageId collisions recursively? It
//       shouldn't be a problem, but might be cool to explore as an exercise.
//    -> extend fnv hash size
func (c *Convo) CreateMessage(data []byte) (string, error) {
	var (
		err error
		// messageId will be populated with the new messageId
		messageId string
		ok        bool
	)

	// attempt to generate a new messageId using the data as salt
	if messageId, err = NewId(data); err != nil {
		return "", err
	}

	// check if a message with the newly generated messageId already exists,
	// because a messageId collision would be bad
	if _, ok = c.Messages[messageId]; ok {
		return "", errors.New("message id overwrite")
	}

	// add the new message to the conversation message map
	c.Messages[messageId] = data

	return messageId, nil
}

// ReadMessage simply retrieves the raw data from a messageId.
func (c *Convo) ReadMessage(messageId string) []byte {
	return c.Messages[messageId]
}

// AddMessage notifies each user in the conversation when a message has been
// added. It returns an error if c.CreateMessage doesn't work with the data
// provided in the params.
func (c *Convo) AddMessage(data []byte, ip string) error {
	var (
		err error
		// messageId will be populated with the new unique id of the message
		messageId string
		// notify sends a notification message to a user and determines whether
		// or not it is coming from them or not (by checking IP)
		notify = func(user *User) {
			var self string
			// if the message is from self, start the line with " ", if it is
			// coming from someone else, start the line with "+" to indicate
			// a new message has been added to the conversation
			if self = "  "; user.IP != ip {
				self = "+ "
			}
			// write the new message notification to the user directly
			user.Write([]byte(self + URL + c.ConvoId + "/" + messageId))
		}
	)

	// attempt to create a new message with the provided data and store the new
	// messageId, otherwise return the error
	if messageId, err = c.CreateMessage(data); err != nil {
		return err
	}

	// notify users that are present in the conversation
	if c.Users[0] != nil {
		notify(c.Users[0])
	}
	if c.Users[1] != nil {
		notify(c.Users[1])
	}

	return nil
}

// Broadcast sends data to each user in the conversation. It returns an error
// if there are no users in the conversation, which hopefully never happens
// because that would mean conversations aren't being deleted properly.
func (c *Convo) Broadcast(data []byte) error {
	// check if there are no users in the conversation, which would be bad
	if c.Users[0] == nil && c.Users[1] == nil {
		return errors.New("no users in conversation")
	}

	// write to each user if they are present in the conversation
	if c.Users[0] != nil {
		c.Users[0].Write(data)
	}
	if c.Users[1] != nil {
		c.Users[1].Write(data)
	}

	return nil
}
