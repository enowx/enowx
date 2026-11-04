// Command cmd runs the stand-in Messages API as a small server for end-to-end tests:
// ANTHROPIC_BASE_URL=http://127.0.0.1:<port> and any ANTHROPIC_API_KEY.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/enowx/enowx/internal/standinapi"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "address to listen on")
	flag.Parse()

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "standinapi:", err)
		os.Exit(1)
	}
	fmt.Printf("http://%s\n", listener.Addr())
	if err := http.Serve(listener, standinapi.NewUnstarted().Handler()); err != nil {
		fmt.Fprintln(os.Stderr, "standinapi:", err)
		os.Exit(1)
	}
}
