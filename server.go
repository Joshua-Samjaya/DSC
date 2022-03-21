package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	PORT := ":2888"
	fmt.Println("Server alive. Listening on port", PORT)

	l, err := net.Listen("tcp4", PORT)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer l.Close()

	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		go handleConnection(c)
	}
}

func handleConnection(c net.Conn) {
	fmt.Printf("Serving %s\n", c.RemoteAddr().String())
	for {
		netData, err := bufio.NewReader(c).ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}

		temp := strings.TrimSpace(string(netData))
		if temp == "STOP" {
			fmt.Println("Exiting TCP server!")
			break
		}

		command := strings.Split(temp, " ")
		if command[0] == "make" {
			if _, err := os.Stat(command[1]); !os.IsNotExist(err) {
				reply := "Path already exist\n"
				c.Write([]byte(reply))
			} else {
				err := os.MkdirAll(command[1], os.ModePerm)
				if err != nil && !os.IsExist(err) {
					log.Fatal(err)
				}
				reply := "Node created\n"
				c.Write([]byte(reply))
			}

		} else if command[0] == "delete" {
			if _, err := os.Stat(command[1]); os.IsNotExist(err) {
				reply := "Path does not exist\n"
				c.Write([]byte(reply))
			} else {
				err := os.RemoveAll(command[1])
				if err != nil {
					log.Fatal(err)
				}
				reply := "Node deleted\n"
				c.Write([]byte(reply))
			}
		} else {
			reply := "Unrecognised command: " + string(netData)
			c.Write([]byte(reply))
		}

	}
	c.Close()
}
