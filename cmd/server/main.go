// Local development server: the same http.Handler the Lambda runs, served
// with net/http so the API can be exercised without deploying.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"flashcard_lambda/internal/app"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	handler, err := app.NewHandler(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
