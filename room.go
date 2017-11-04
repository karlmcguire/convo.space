package main

import (
	"errors"
	"fmt"
	"sync"
)

// Room contains multiple conversations and a mutex for safety.
type Room struct {
	sync.Mutex
	// Convos is a map of all active conversations where the key is convoId
	Convos map[string]*Convo
}

// IPExists determines whether or not one of the users in the conversation has
// the ip passed as a parameter. This is used to make sure that no one other
// than the conversation participants can read/write messages.
func (r *Room) IPExists(convoId, ip string) bool {
	r.Lock()
	defer r.Unlock()

	if r.Convos[convoId].Users[0] != nil &&
		r.Convos[convoId].Users[0].IP == ip {
		return true
	} else if r.Convos[convoId].Users[1] != nil &&
		r.Convos[convoId].Users[1].IP == ip {
		return true
	}

	return false
}

// OtherUser returns a notification of the other user's IP in a conversation.
// This is used when a user is joining a conversation with someone else already
// waiting for them. This way you can know the IP of who's on the other side
// even if you weren't there to see them join (and read the join notification).
func (r *Room) OtherUser(convoId string, userId int) []byte {
	r.Lock()
	defer r.Unlock()

	// return the notification message with the other user's ip
	return []byte(fmt.Sprintf(
		"> %s",
		r.Convos[convoId].Users[OtherUserId(userId)].IP),
	)
}

// DeleteUser removes the user from a conversation and deletes the user.
func (r *Room) DeleteUser(convoId string, userId int) {
	r.Lock()
	defer r.Unlock()

	// get the user ip for the quit message later
	ip := r.Convos[convoId].Users[userId].IP

	// delete the user from the conversation
	r.Convos[convoId].Users[userId] = nil

	// if this user is the last one leaving a conversation, also end the
	// conversation and delete it
	if r.Convos[convoId].Users[0] == nil &&
		r.Convos[convoId].Users[1] == nil {

		println("deleting " + convoId)

		// stop the pinging service
		r.Convos[convoId].Stop <- struct{}{}
		// remove the conversation from the room
		delete(r.Convos, convoId)

		return
	}

	// write the user leaving notification to the remaining user
	r.Convos[convoId].Users[OtherUserId(userId)].Write([]byte(
		"< " + ip,
	))
}

// ReadMessage returns the raw data of the message with messageId, and deletes
// the message from the conversation.
//
// TODO: Add information to the message-read notification (like IP and time).
//    -> see main.go for possible IP checks
func (r *Room) ReadMessage(convoId, messageId string) ([]byte, error) {
	r.Lock()
	defer func() {
		// delete the message before unlocking mutex
		delete(r.Convos[convoId].Messages, messageId)
		r.Unlock()
	}()

	// check if the message exists
	if r.Convos[convoId].ReadMessage(messageId) == nil {
		return nil, errors.New("message doesn't exist")
	}

	// broadcast that the message was read
	r.Convos[convoId].Broadcast(
		[]byte("- " + URL + convoId + "/" + messageId),
	)

	// return the raw content of the message
	return r.Convos[convoId].ReadMessage(messageId), nil
}

// AddMessage adds a new message to the conversation.
func (r *Room) AddMessage(data []byte, convoId, ip string) error {
	r.Lock()
	defer r.Unlock()

	return r.Convos[convoId].AddMessage(data, ip)
}

// JoinConvo adds a user to a conversation.
func (r *Room) JoinConvo(user *User, convoId string) error {
	r.Lock()
	defer r.Unlock()

	// assign the user's convoId to the new convoId
	user.ConvoId = convoId

	if r.Convos[convoId].Users[0] == nil &&
		r.Convos[convoId].Users[1] != nil {
		// if someone is in the 1 slot assign the new user to the 0 slot
		user.UserId = 0
	} else if r.Convos[convoId].Users[1] == nil &&
		// if someone is in the 0 slot assign the new user to the 1 slot
		r.Convos[convoId].Users[0] != nil {
		user.UserId = 1
	} else {
		// this is very bad
		return errors.New("this is bad")
	}

	// broadcast to the conversation that someone joined
	r.Convos[convoId].Broadcast([]byte(fmt.Sprintf("> %s", user.IP)))
	// assign the new user to the conversation
	r.Convos[convoId].Users[user.UserId] = user

	return nil
}

// CreateConvo creates a new conversation with the user.
//
// TODO: More convoId collision checks/solutions?
func (r *Room) CreateConvo(user *User) (string, error) {
	var (
		err error
		// convoId will be populated with the new unique conversation id
		convoId string
	)

	// attempt to create a new convoId and return the error if it fails
	if convoId, err = NewId(nil); err != nil {
		return "", err
	}

	r.Lock()
	defer r.Unlock()

	// check if there was a collision
	if _, ok := r.Convos[convoId]; ok {
		return "", errors.New("convo id overwrite")
	}

	// assign the new user to the new conversation
	user.ConvoId = convoId
	// this user is the first one
	user.UserId = 0

	// add the convo to the room map
	r.Convos[convoId] = &Convo{
		ConvoId:  convoId,
		Users:    [2]*User{user, nil},
		Messages: make(map[string][]byte, 0),
		Stop:     make(chan struct{}),
	}

	// start the ping goroutine
	go r.Convos[convoId].Ping()

	println("creating " + convoId)

	return convoId, nil
}

// IsConvo determines whether a conversation exists or not.
func (r *Room) IsConvo(convoId string) bool {
	r.Lock()
	defer r.Unlock()

	_, ok := r.Convos[convoId]
	return ok
}

// IsConvoFull determines whether a conversation is full (2 users) or not (1
// user). It is expected that IsConvo is called before IsConvoFull, because
// IsConvoFull just assumes a conversation exists with convoId.
//
// TODO: How to handle convoId's that don't exist?
//    -> right now just checking in main.go
func (r *Room) IsConvoFull(convoId string) bool {
	r.Lock()
	defer r.Unlock()

	return r.Convos[convoId].Users[0] != nil &&
		r.Convos[convoId].Users[1] != nil
}
