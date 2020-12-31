package main

import (
	"bitbucket.org/jhalter/hotline"
	"flag"
	"log"
)

var hotlineServer *hotline.Server

func main() {
	basePort := flag.Int("bind", 5500, "Bind address and port")
	configDir := flag.String("config", "config/", "Path to config root")
	//logLevel := flag.String("log-level", "info", "Log level")
	flag.Parse()

	log.Fatal(hotline.ListenAndServe(*basePort, *configDir))
}

