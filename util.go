package main

import (
	"fmt"
	"hash/fnv"
	"net"
	"time"
)

// OtherUserId simply returns the id of the opposite user.
func OtherUserId(userId int) int {
	return (^userId) + 2
}

// GetIP simply cleans up a raw IP string.
// (Removes socket number.)
func GetIP(ip string) string {
	host, _, _ := net.SplitHostPort(ip)
	return host
}

// NewId creates a new unique ID with data as the salt.
func NewId(data []byte) (string, error) {
	var (
		now   []byte
		err   error
		fhash = fnv.New32a()
	)

	// get the current time
	if now, err = time.Now().MarshalBinary(); err != nil {
		return "", err
	}

	// append the salt to the time
	fhash.Write(append(now[:], data[:]...))

	return fmt.Sprintf("%d", fhash.Sum32()), nil
}
