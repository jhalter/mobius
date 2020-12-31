package main

import (
	"bitbucket.org/jhalter/hotline"
	"flag"
)

func main() {
	login := flag.String("login", "", "login name")
	passwd := flag.String("passwd", "", "login password")
	username := flag.String("username", "unnamed", "User name")
	flag.Parse()

	c := hotline.NewClient(*username)
	c.JoinServer("localhost:5600", *login, *passwd)
	defer c.Disconnect()

	c.Read()

}
