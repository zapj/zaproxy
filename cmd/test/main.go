package main

import (
	"crypto/tls"
	"log"
	"net/http"
)

func main() {
	server := &http.Server{
		Addr: ":8888",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				w.Write([]byte("hello tls"))
			} else {
				w.Write([]byte("hello http"))
			}
		}),
		// Disable HTTP/2.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	log.Fatal(server.ListenAndServeTLS("data/server.crt", "data/server.key"))

}
