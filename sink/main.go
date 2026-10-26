// Command sink is the local "exfiltration" endpoint. It listens on 127.0.0.1:4444
// (override with SINK_ADDR) and prints whatever the fake module sends it, so you
// can show on stage that the token bytes really left the process — while nothing
// ever leaves your machine.
package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
)

func main() {
	addr := "127.0.0.1:4444"
	if v := os.Getenv("SINK_ADDR"); v != "" {
		addr = v
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sink listening on %s", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func(c net.Conn) {
			defer c.Close()
			b, _ := io.ReadAll(c)
			fmt.Printf("\n── received %d bytes from %s ──\n%s\n", len(b), c.RemoteAddr(), b)
		}(conn)
	}
}
