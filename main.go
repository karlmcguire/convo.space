package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	// some messages need https://DOMAIN:PORT/
	DEFAULT_DOMAIN = "localhost"
	DEFAULT_PORT   = 8080

	// filepath locations of the needed SSL files
	DEFAULT_CERT_LOCATION = "../ssl/cert.pem"
	DEFAULT_KEY_LOCATION  = "../ssl/key.pem"

	// only used if localhost
	URL_PORT_FORMAT = "https://%s:%d/"

	// used for real domains
	URL_FORMAT = "https://%s/"
)

var (
	// Store is the global store of all the conversations.
	Store *Room = &Room{Convos: make(map[string]*Convo, 0)}
	// SSL config stuff
	TLSCONFIG = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		CurvePreferences: []tls.CurveID{
			tls.CurveP521,
			tls.CurveP384,
			tls.CurveP256,
		},
		PreferServerCipherSuites: true,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
	}

	// URL is the final https://DOMAIN:PORT/ string to be sent in messages
	URL string
)

// GET is called when someone makes a GET request to the server. This function
// first determines whether or not it is coming from curl, and then determines
// the user's intention based on URL variables.
func GET(w http.ResponseWriter, r *http.Request, ids []string) {
	// if the request isn't coming from CURL then we want to display the
	// landing page
	//
	// if the request is coming from curl then we want to check the URL
	// variables and handle accordingly
	if len(r.Header.Get("User-Agent")) < 4 ||
		r.Header.Get("User-Agent")[:4] != "curl" {
		// write the landing page
		w.Write([]byte(PAGE))
		return
	}

	if len(ids) == 2 {
		if len(ids[1]) == 0 { // https://DOMAIN/
			var (
				user    *User = NewUser(w, r)
				convoId string
				err     error
			)

			// attempt to create a new conversation and store the convoId
			if convoId, err = Store.CreateConvo(user); err != nil {
				panic(err)
			}

			// write the new link to the initial user
			go user.Write([]byte(": " + URL + convoId))

			// start the listening
			if err = user.Listen(); err != nil {
				panic(err)
			}
		} else { // https://DOMAIN/convoId
			// the client is trying to join a conversation with convoId
			var (
				user    *User  = NewUser(w, r)
				convoId string = ids[1]
				err     error
			)

			// check if the conversation exists and whether it's full
			if !Store.IsConvo(convoId) || Store.IsConvoFull(convoId) {
				return
			}

			// attempt to add the new user to the conversation
			if err = Store.JoinConvo(user, convoId); err != nil {
				panic(err)
			}

			// since user.Listen() will run infinitely, we need to add a
			// notification message to the user's message queue before calling
			// user.Listen(), because user.Write will block until something
			// can read from the channel
			//
			// this small goroutine will fire once
			go user.Write(Store.OtherUser(convoId, user.UserId))

			// start the listening
			if err = user.Listen(); err != nil {
				panic(err)
			}

			// the user.Write above will fire here
		}
	} else if len(ids) == 3 { // https://DOMAIN/convoId/messageId
		var (
			convoId   string = ids[1]
			messageId string = ids[2]
			data      []byte
			err       error
		)

		// check if the conversation actually exists
		if !Store.IsConvo(convoId) {
			return
		}

		// TODO: is this needed?
		if !Store.IPExists(convoId, GetIP(r.RemoteAddr)) {
			return
		}

		// attempt to read the message
		if data, err = Store.ReadMessage(convoId, messageId); err != nil {
			panic(err)
		}

		// write the raw data out to the client
		w.Write(data)
	}
}

// PUT is called when someone sends a PUT request to the server. This function
// determines whether or not the request to add a message is valid and if so,
// adds the message to the specified conversation.
func PUT(w http.ResponseWriter, r *http.Request, ids []string) {
	if len(ids) == 2 { // https://DOMAIN/convoId
		var (
			convoId string = ids[1]
			data    []byte
			err     error
		)

		// make sure a conversation with the convoId actually exists
		if !Store.IsConvo(convoId) {
			return
		}

		// TODO: is this needed?
		if !Store.IPExists(convoId, GetIP(r.RemoteAddr)) {
			return
		}

		// read the data from the request body
		if data, err = ioutil.ReadAll(r.Body); err != nil {
			panic(err)
		}

		// attempt to add the message to the conversation
		if err = Store.AddMessage(
			data,
			convoId,
			GetIP(r.RemoteAddr),
		); err != nil {
			panic(err)
		}
	}
}

func main() {
	var (
		domainPtr = flag.String(
			"domain",
			DEFAULT_DOMAIN,
			"domain people use to reach this server",
		)
		portPtr = flag.Int(
			"port",
			DEFAULT_PORT,
			"port number to listen on",
		)
		certPtr = flag.String(
			"cert",
			DEFAULT_CERT_LOCATION,
			"SSL certificate filepath",
		)
		keyPtr = flag.String(
			"key",
			DEFAULT_KEY_LOCATION,
			"SSL key filepath",
		)
	)

	flag.Parse()

	// only add the port to the url if the domain is localhost
	//
	// TODO: find a better way to do this? maybe another flag?
	if *domainPtr != "localhost" {
		URL = fmt.Sprintf(URL_FORMAT, *domainPtr)
	} else {
		URL = fmt.Sprintf(URL_PORT_FORMAT, *domainPtr, *portPtr)
	}

	var (
		err    error
		mux    *http.ServeMux = http.NewServeMux()
		server http.Server    = http.Server{
			Addr:      fmt.Sprintf(":%d", *portPtr),
			Handler:   mux,
			TLSConfig: TLSCONFIG,
			TLSNextProto: make(map[string]func(
				*http.Server,
				*tls.Conn,
				http.Handler,
			), 0),
		}
	)

	// this handles all incoming requests and routes them to GET or PUT
	// accordingly
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Path, "/")
		switch r.Method {
		case "GET":
			GET(w, r, ids)
		case "PUT":
			PUT(w, r, ids)
		}
	})

	println("listening on " + URL)

	if err = server.ListenAndServeTLS(*certPtr, *keyPtr); err != nil {
		panic(err)
	}
}
