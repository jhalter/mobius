package main

import (
	"bitbucket.org/jhalter/hotline"
	"flag"
	"github.com/getsentry/sentry-go"
	"log"
	"time"
)

var hotlineServer *hotline.Server

func main() {
	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://f21adf4c207c449687dda034709f6b2e@o258088.ingest.sentry.io/5831364",
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	// Flush buffered events before the program terminates.
	defer sentry.Flush(2 * time.Second)

	basePort := flag.Int("bind", 5500, "Bind address and port")
	configDir := flag.String("config", "config/", "Path to config root")
	//logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	log.Fatal(hotline.ListenAndServe(*basePort, *configDir))
}

