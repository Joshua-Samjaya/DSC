package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

type ServerRole string
var myRole ServerRole
var myIP string

// const LOG_FILE = "./mytxn.txt"

var listIP []string = []string {"10.10.10.14"}
const NUM_PEERS = 1
const QUORUM = 1

var listConn []net.Conn
var leaderConn net.Conn
var clientConn net.Conn

var txnlog *log.Logger

const (
	Undefined ServerRole = ""
	Leader 				 = "LEADER"
	Follower			 = "FOLLOWER"
)

type TxnId struct {
	epoch int
	counter int
}

func main() {
	roleFlag := flag.String("role", "LEADER", "Must be one of LEADER or FOLLOWER")
	flag.Parse()
	myRole = ServerRole(*roleFlag)

	LOG_FILE := "./mytxn_"+*roleFlag+".txt"
	txnFile, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        log.Panic(err)
    }
    defer txnFile.Close()
	txnlog = log.New(txnFile, "", log.Ldate|log.Ltime|log.Lshortfile)

	connectToPeers()

	//FROM CLIENTS
	// PORT := ":2888"
	var PORT string
	if myRole == Leader { PORT = ":2888" 
	} else { PORT = ":2889" }

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

func connectToPeers() {
	port := ":8080"
	switch myRole {
	case Leader:
		done := 0
		l, err := net.Listen("tcp4", port)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer l.Close()
		for {
			c, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				done -= 1
				return
			}
			listConn = append(listConn, c)
			fmt.Printf("Knows %s\n", c.RemoteAddr().String())
			go leaderBroadcast(c)
			done+=1
			if done == NUM_PEERS { return }
		}

	case Follower:
		//replace with initial leader address
		leader := "0.0.0.0"
		addr := leader + port

		c, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Println(err)
			return
		}
		leaderConn = c
		go followerBroadcast(c)
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
				switch myRole {
				case Follower:
					leaderConn.Write([]byte(temp+"\n"))

				case Leader:
					err := os.MkdirAll(command[1], os.ModePerm)
					if err != nil && !os.IsExist(err) {
						log.Fatal(err)
					}
					reply := "Node created\n"
					c.Write([]byte(reply))
				}
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


func followerBroadcast(c net.Conn) {
	/*
	receive msg PROPOSE
	send msg ACK
	receive msg COMMIT
	*/
	reader := bufio.NewReader(c)
	for {
		bcMsg, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		// fmt.Println(bcMsg)
		bcMsg = strings.TrimSpace(string(bcMsg))
		temp := strings.Split(bcMsg, " ")

		if temp[0] == "PROPOSE" {
			txnlog.Println(bcMsg)
			sendMsg := "ACK"+" "+temp[1]+" "+temp[2]+"\n"
			c.Write([]byte(sendMsg))
		} else if temp[0] == "COMMIT" {
			//TODO: execute command
			fmt.Println(temp)
			txnlog.Println(bcMsg)
			executeCommand(temp[1:])
		}	
		bcMsg = ""
		temp = nil
	}
}

func leaderBroadcast(c net.Conn) {
	/*
	send msg PROPOSE
	receive msg ACK
	send msg COMMIT
	*/
	replies := 0
	reader := bufio.NewReader(c)
	for {
		bcMsg, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		// fmt.Println(bcMsg)
		bcMsg = strings.TrimSpace(string(bcMsg))
		temp := strings.Split(bcMsg, " ")
		var cmd string

		if temp[0] == "ACK" {
			replies += 1
			if replies >= QUORUM {
				// fmt.Println(cmd)
				// sendMsg := "COMMIT "+cmd+"\n"
				sendMsg := "COMMIT"+" "+temp[1]+" "+temp[2]+"\n"
				c.Write([]byte(sendMsg))

				txnlog.Println(sendMsg)
				replies = 0
				executeCommand(temp[1:])
				cmd=""
			}
		} else if len(temp)>0 {
			cmd = bcMsg
			fmt.Println(cmd)
			sendMsg := "PROPOSE "+bcMsg+"\n"
			txnlog.Println(sendMsg)
			c.Write([]byte(sendMsg))
		}
		bcMsg = ""
		temp = nil
	}
}

func executeCommand(command []string) {
	if command[0] == "make" {
		if _, err := os.Stat(command[1]); !os.IsNotExist(err) {
			fmt.Println("Path already exist\n")
		} else {
		
			err := os.MkdirAll(command[1], os.ModePerm)
			if err != nil && !os.IsExist(err) {
				log.Fatal(err)
			}
			fmt.Println("Node created\n")
		}

	} else if command[0] == "delete" {
		if _, err := os.Stat(command[1]); os.IsNotExist(err) {
			fmt.Println("Path does not exist\n")
		} else {
			err := os.RemoveAll(command[1])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Node deleted\n")
		}
	} else {
		fmt.Println("Unrecognised command")
	}
}