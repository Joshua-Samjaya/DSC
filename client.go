package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {

	fmt.Println("Client alive")

	// replace with server address
	server := "0.0.0.0"
	port := "2888"
	addr := server + ":" + port

	c, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Println(err)
		return
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(">> ")
		text, _ := reader.ReadString('\n')

		s := strings.Split(text[0:len(text)-2], " ")
		if (s[0] == "make" || s[0] == "delete") && len(s) < 2 {
			fmt.Println("Please provide a valid argument!")
			continue
		}
		fmt.Fprintf(c, text+"\n")

		message, _ := bufio.NewReader(c).ReadString('\n')
		fmt.Print("->: " + message)
		if strings.TrimSpace(string(text)) == "STOP" {
			fmt.Println("TCP client exiting...")
			return
		}
	}
}
