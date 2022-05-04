package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func main() {

	if len(os.Args) == 1 {
		fmt.Println("Too little parameters! Please only run as:")
		fmt.Println("go run testclient.go <command_file>")
		os.Exit(1)
	}

	if len(os.Args) != 2 {
		fmt.Println("Too many parameters! Please only run as:")
		fmt.Println("go run testclient.go <command_file>")
		os.Exit(1)
	}

	cmd_file := os.Args[1]
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

	file, err := os.Open(cmd_file)

	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	for {
		scanner := bufio.NewScanner(file)

		// for loop to send requests from specified command input file
		for scanner.Scan() {
			// 10 requests per second
			time.Sleep(time.Millisecond * 100)

			text := scanner.Text()
			fmt.Println(scanner.Text())
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

}

//consume response from server
func handleClientConn(c net.Conn) {
	for {
		netData, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Print("From server:", netData)
		fmt.Print(">> ")
	}
}
