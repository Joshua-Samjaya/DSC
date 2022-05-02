package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"
	"flag"
	"os"
	"strings"
	"math"
	"sort"
	"strconv"
)

type ServerRole string
const (
	Undefined ServerRole = ""
	Leader 				 = "LEADER"
	Follower			 = "FOLLOWER"
)

type TxnId struct {
	epoch int
	counter int
}

type Transaction struct {
	id TxnId
	req string
	replies int
}


type Server struct{
	ip string
	role ServerRole
	coordinator string
	listeningport string
	electing bool
	waiting bool
	waitresponse bool
	timedout string
	myChannel chan string
	listConn map[string]*net.Conn
	leaderConn net.Conn
	electionConn map[string]*net.Conn
	highestTxnId TxnId
	quorum int
	txnFile *os.File
	nodes int
	queue []string
	txnReplies map[string]int
	txnlog *log.Logger
}

func (s *Server) connectToPeers(){
	port := ":8080"
	switch s.role {

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
				return
			}
			if s.role == Follower{
				return
			}
			if addr, ok := c.RemoteAddr().(*net.TCPAddr); ok {
				for i, _ := range s.listConn {
					fmt.Println("Sending node message to", i)
					fc := *s.listConn[i]
					fc.Write([]byte("NODE " + addr.IP.String() + "\n"))
					c.Write([]byte("NODE " + i + "\n"))
				}
				s.listConn[addr.IP.String()] = &c
				newc, err := net.Dial("tcp", addr.IP.String() + ":2888")
				if err != nil{
					fmt.Println(err)
				}
				s.electionConn[addr.IP.String()] = &newc
			}
			done+=1
			fmt.Printf("Knows %s, connected = %d/%d\n", c.RemoteAddr().String(), done, s.nodes-1)
			go s.leaderBroadcast(c)
			
			// if done == s.nodes-1 { return }
		}
		fmt.Println("All followers connected")
		

	case Follower:
		addr := s.coordinator + port
		c, err := net.Dial("tcp", addr)
		if err != nil {
			fmt.Println(err)
			return
		}
		s.leaderConn = c
		go s.followerBroadcast(c)
	}
}

//This function is run on a separate thread whenever a message is sent towards the coordinator, that after 2 seconds if the coordinator does not respond, will start an election
func (s *Server) messagetimer() {
	timer := time.NewTimer(2 * time.Second)
	<- timer.C
	if s.waitresponse{
		fmt.Println(s.ip, "message to coordinator timed out") //For debugging/demo purposes
		go s.startelection()
	}
}

//This function is called after the timeout of an expected coordinator response
func (s *Server) startelection(){
	s.electing = true
	fmt.Println(s.ip, "Starting Election") //For debugging/demo purposes
	for ip, _ := range s.electionConn {
		go s.sendelection(ip)
	}
}

//This function is run on a separate thread to send the election messages towards all nodes with higher IP
func (s *Server) sendelection(ip string){
	electionMessage := "COORDINATOR " + s.ip + " EPOCH " + strconv.Itoa(s.highestTxnId.epoch) + " COUNTER " + strconv.Itoa(s.highestTxnId.counter) + "\n"
	fmt.Fprintf(*s.electionConn[ip], electionMessage)
	go s.electiontimer()
}

//This function is run on a separate thread to wait for each sent election message to either receive a response or not. When a response is not received within the time window, the timer will count down the required timeouts
//When the election counter reaches 0, the node will declare victory 
func (s *Server) electiontimer(){
	timer := time.NewTimer(2 * time.Second)
	<- timer.C
	if s.electing{
		s.declarevictory()
	}
}

//This functions declares victory to all servers
func (s *Server) declarevictory(){
	s.electing = false
	fmt.Println(s.ip, "declaring victory") //For debugging/demo purposes
	s.coordinator = s.ip
	s.role = Leader
	s.leaderConn.Close()
	for ip, _ := range s.listConn{
		c := *s.listConn[ip]
		c.Close()
	}
	go s.connectToPeers()
	victoryMessage := "VICTORY " + s.ip + "\n"
	for ip, _ := range s.electionConn{
		c := *s.electionConn[ip]
		c.Write([]byte(victoryMessage))
	}
}

//This function is used to dynamically allocate the IP to the server struct
func GetOutboundIP() string {
    conn, err := net.Dial("udp", "8.8.8.8:80")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    localAddr := conn.LocalAddr().(*net.UDPAddr)

    return localAddr.IP.String()
}

//A checker for a certain string within a slice
func stringInSlice(a string, list []string) bool {
    for _, b := range list {
        if b == a {
            return true
        }
    }
    return false
}

//This function is called on separate threads whenever a connection comes into the specified port
func (s *Server) handleConnection(c net.Conn) {
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
		if command[0] == "make" || command[0] == "delete" || command[0] == "write"{
			if command[0] == "make" {
				if _, err := os.Stat(command[1]); !os.IsNotExist(err) {
				reply := "Path already exist\n"
				c.Write([]byte(reply))
				} else {
					s.startBroadcast(temp)
				}
			} else if command[0] == "delete" || command[0] == "write" {
				if _, err := os.Stat(command[1]); os.IsNotExist(err) {
				reply := "Path does not exist\n"
				c.Write([]byte(reply))
				} else {
					s.startBroadcast(temp)
				}
			} 
		} else if command[0] == "read" {
			if _, err := os.Stat(command[1]); os.IsNotExist(err) {
				fmt.Println("Path does not exist")
				reply := "Path does not exist\n"
				c.Write([]byte(reply))
			} else {
				f, err := os.Open(command[1] + "/data")
				if err != nil {
					log.Fatal(err)
				}
				dat := make([]byte, 20)
				_, err2 := f.Read(dat)
				if err2 != nil {
					log.Fatal(err)
				}
				f.Close()
				datStr := string(dat)
				fmt.Println(datStr)
				c.Write([]byte(datStr + "\n"))
			}
		} else if command[0] == "COORDINATOR" {
			fmt.Println("Server", s.ip, "recevied message:", temp)
			epoch, _ := strconv.Atoi(command[3])
			counter, _ := strconv.Atoi(command[5])
			if epoch < s.highestTxnId.epoch{
				if !s.electing && !s.waiting{
					go s.startelection()
					s.electing = true
				}
			} else if epoch == s.highestTxnId.epoch{
				if counter < s.highestTxnId.counter{
					if !s.electing && !s.waiting{
						go s.startelection()
						s.electing = true
					}
				} else if counter == s.highestTxnId.counter{
					if strings.Compare(s.ip, command[1]) == 1{
						if !s.electing && !s.waiting{
							go s.startelection()
							s.electing = true
						}
					} else{
						if s.electing{
							s.electing = false
							s.waiting = true
						}else{
							s.waiting = true
						}
					}
				} else{
					if s.electing{
						s.electing = false
						s.waiting = true
					}else{
						s.waiting = true
					}
				}
			} else{
				if s.electing{
					s.electing = false
					s.waiting = true
				}else{
					s.waiting = true
				}
			}
		} else if command[0] == "VICTORY"{
			fmt.Println("Server", s.ip, "recevied message:", temp)
			s.electing = false
			s.waiting = false
			s.coordinator = command[1]
			s.role = Follower
			s.leaderConn.Close()
			for ip, _ := range s.listConn{
				c := *s.listConn[ip]
				c.Close()
			}
			go s.connectToPeers()
			if s.timedout != ""{
				time.Sleep(1 * time.Second)
				s.startBroadcast(s.timedout)
			}
		} else {
			reply := "Unrecognised command: " + string(netData)
			c.Write([]byte(reply))
		}

	}
	c.Close()
}


func (s *Server) startBroadcast(temp string) {
	switch s.role {
	case Follower: 
		c := s.leaderConn
		s.timedout = temp
		s.waitresponse = true
		go s.messagetimer()
		c.Write([]byte(temp+"\n"))

	case Leader:
		s.myChannel <- temp+"\n"
	}
}
func newTransaction(id TxnId, req string) (Transaction) {
	txn := Transaction{
		id:		id,
		req:	req,
	}
	return txn
}

func lessThan (a *TxnId, b *TxnId) bool {
	return (a.epoch <= b.epoch && a.counter < b.counter)
}

func enqueue(q []string, id string) []string {
	q = append(q, id)
	//priority queue based on TxnId
	if len(q) > 1 {
		sort.Slice(q, func(i, j int) bool {
			var byE, byC bool
			iList := strings.Split(q[i], "-")
			jList := strings.Split(q[j], "-")
			e1, _ := strconv.Atoi(iList[0]); c1, _ := strconv.Atoi(iList[1])
			e2, _ := strconv.Atoi(jList[0]); c2, _ := strconv.Atoi(jList[1])
			byE = e1<e2
			if e1 == e2 {
				byC = c1<c2
				return byC
			}
			return byE
		})
	}
	return q
}

func (s *Server) followerBroadcast(c net.Conn) {
	reader := bufio.NewReader(c)
	for {
		bcMsg, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		bcMsg = strings.TrimSpace(string(bcMsg))
		temp := strings.Split(bcMsg, " ")

		if temp[0] == "PROPOSE" {
			// queue = append(queue, temp[1])	//enqueue
			txnid := strings.Split(temp[1], "-")
			epoch, _ := strconv.Atoi(txnid[0])
			counter, _ := strconv.Atoi(txnid[1])
			if epoch > s.highestTxnId.epoch{
				s.highestTxnId = TxnId{epoch, counter}
			} else if epoch == s.highestTxnId.epoch{
				if counter > s.highestTxnId.counter{
					s.highestTxnId = TxnId{epoch, counter}
				}
			}
			s.timedout = ""
			s.waitresponse = false
			s.queue = enqueue(s.queue, temp[1])
			s.txnlog.Println(bcMsg)
			sendMsg := "ACK"
			sendMsg += " "+strings.Join(temp[1:]," ")+"\n"
			c.Write([]byte(sendMsg))

		} else if temp[0] == "COMMIT" {
			if len(s.queue)>0 {s.queue = s.queue[1:]} //dequeue
			s.waitresponse = false
			s.txnlog.Println(bcMsg)
			fmt.Print("Committing: " + strings.Join(temp, " "))
			executeCommand(temp[2:])
		} else if temp[0] == "NODE"{
			newc, err := net.Dial("tcp", temp[1] + ":2888")
			if err != nil{
				fmt.Println(err)
				continue
			}
			s.electionConn[temp[1]] = &newc
		}
		bcMsg = ""
		temp = nil
	}
}

func strTxnId(id TxnId) string {
	return fmt.Sprintf("%d-%d", id.epoch, id.counter)
}
func (s *Server) leaderBroadcast(c net.Conn) {
	reader := bufio.NewReader(c)
	var bcMsg string
	for {
		select {
		case ownClientReq := <- s.myChannel:
			fmt.Println(ownClientReq)
			bcMsg = strings.TrimSpace(string(ownClientReq))

		default:
			fwd, err := reader.ReadString('\n')
			if err != nil {
				fmt.Println(err)
				return
			}
			bcMsg = strings.TrimSpace(string(fwd))
		}

		temp := strings.Split(bcMsg, " ")
		

		if temp[0] == "ACK" {
			s.txnReplies[temp[1]] +=1 
			replies := s.txnReplies[temp[1]]
			//TODO: remove from map?
			if replies == s.quorum {
				sendMsg := "COMMIT"
				sendMsg += " "+strings.Join(temp[1:]," ")+"\n"
				// c.Write([]byte(sendMsg))
				for i, _ := range s.listConn {
					fc := *s.listConn[i]
					fc.Write([]byte(sendMsg))
				}
				s.txnlog.Println(sendMsg)
				replies = 0
				executeCommand(temp[2:])
			}
		} else if len(temp)>0 {
			//convert request to transaction
			id := TxnId{s.highestTxnId.epoch, s.highestTxnId.counter+1}
			s.highestTxnId = id
			s.txnReplies[strTxnId(id)] = 1

			sendMsg := "PROPOSE"+" "+strTxnId(id)+" "+bcMsg+"\n"
			s.txnlog.Println(sendMsg)
			// c.Write([]byte(sendMsg))
			for i, _ := range s.listConn {
				fc := *s.listConn[i]
				fc.Write([]byte(sendMsg))
			}
		}
		bcMsg = ""
		temp = nil
	}
}

func executeCommand(command []string) {
	if command[0] == "make" {
		if _, err := os.Stat(command[1]); !os.IsNotExist(err) {
			fmt.Println("Path already exist")
		} else {
			err := os.MkdirAll(command[1], os.ModePerm)
			if err != nil && !os.IsExist(err) {
				log.Fatal(err)
			}
			fmt.Println("Node created")
		}

	} else if command[0] == "delete" {
		if _, err := os.Stat(command[1]); os.IsNotExist(err) {
			fmt.Println("Path does not exist")
		} else {
			err := os.RemoveAll(command[1])
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println("Node deleted")
		}
	} else if command[0] == "write" {
		if _, err := os.Stat(command[1]); os.IsNotExist(err) {
			fmt.Println("Path does not exist")
		} else {
			fmt.Println(command)
			f, err := os.Create(command[1] + "/data")
			if err != nil {
				log.Fatal(err)
			}
			_, err2 := f.Write([]byte(command[2]))
			if err2 != nil {
				log.Fatal(err)
			}
			f.Close()
			fmt.Println("Content written")
		}
	} else {
		fmt.Println("Unrecognised command")
	}
}

func main() {
	localaddr := GetOutboundIP()

	sizeFlag := flag.Int("nodes", 3, "Must be positive integer")
	flag.Parse()
	NUM_NODES := *sizeFlag
	QUORUM := int(math.Floor(float64(NUM_NODES/2)) + 1)
	LOG_FILE := "./mytxn.txt"
	txnFile, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
    if err != nil {
        log.Panic(err)
    }
    defer txnFile.Close()

	server := &Server{
		ip: localaddr,
		coordinator: "10.10.10.13",
		listeningport: ":2888",
		electing: false,
		waiting: false,
		waitresponse: false,
		listConn: make(map[string]*net.Conn),
		myChannel: make(chan string),
		electionConn: make(map[string]*net.Conn),
		highestTxnId: TxnId{0,0},
		quorum: QUORUM,
		txnFile: txnFile,
		nodes: NUM_NODES,
		queue: []string{},
		txnReplies: make(map[string]int),
		txnlog: log.New(txnFile, "", log.Ldate|log.Ltime|log.Lshortfile),
	}
	if localaddr == "10.10.10.13" {
		server.role = Leader
	} else {
		server.role = Follower
	}
	go server.connectToPeers()

	// go server.recv()
	// time.Sleep(1 * time.Second)
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
		fmt.Println("Received new connection from client")
		if addr, ok := c.RemoteAddr().(*net.TCPAddr); ok {
			fmt.Println(addr.IP.String())
		}
		go server.handleConnection(c)
	}
}