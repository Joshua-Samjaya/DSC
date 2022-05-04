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

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Insert server IP: ")
	server, _ := reader.ReadString('\n')
	server = server[:len(server)-1]
	fmt.Println("Trying to connect to", server, "....")
	port := "2888"
	addr := server + ":" + port

	c, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Print("Error occured")
		fmt.Println(err)
		return
	}
	fmt.Println("Successfully connected")

	go handleClientConn(c)

	fmt.Print(">> ")
	for {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		s := strings.Split(text, " ")

		if s[0] != "read" {
			fmt.Print(">> ")
		}

		if (s[0] == "make" || s[0] == "delete" || s[0] == "read") && len(s) < 2 {
			fmt.Println("Please provide a valid argument!")
			continue
		} else if s[0] == "write" && len(s) < 3 {
			fmt.Println("Please provide valid arguments!")
			continue
		}
		fmt.Fprintf(c, text+"\n")

		if strings.TrimSpace(string(text)) == "STOP" {
			fmt.Println("TCP client exiting...")
			return
		}
	}

}

func handleClientConn(c net.Conn) {
	for {
		netData, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Print(netData)
		fmt.Print(">> ")
	}
}
