package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ServerRole string

const (
	Undefined ServerRole = ""
	Leader               = "LEADER"
	Follower             = "FOLLOWER"
)

type TxnId struct {
	epoch   int
	counter int
}

type Transaction struct {
	id  TxnId
	req string
}

type Server struct {
	ip            string
	role          ServerRole
	normal        bool
	coordinator   string
	listeningport string
	electing      bool
	waiting       bool
	waitresponse  bool
	timedout      string
	myChannel     chan string
	listConn      map[string]*net.Conn
	leaderConn    net.Conn
	electionConn  map[string]*net.Conn
	highestTxnId  TxnId
	quorum        int
	txnFile       *os.File
	nodes         int
	queue         []Transaction  //maintained in follower, to keep proposals from server
	txnReplies    map[string]int //maintained in leader, to keep transactions and reply counts from followers
	txnlog        *log.Logger
	reqQueue      []string
	arr_lock      sync.Mutex
	read_lock     sync.Mutex
}

/* --- GLOBAL VARIABLES --- */
var mutex = &sync.RWMutex{}

/* --- GLOBAL VARIABLES END --- */

func (s *Server) connectToPeers() {
	port := ":8080"
	switch s.role {

	case Leader:
		done := 0
		l, err := net.Listen("tcp4", port)
		if err != nil {
			fmt.Println("ERROR", err)
			return
		}
		defer l.Close()
		for {
			c, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				return
			}
			if s.role == Follower {
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
				newc, err := net.Dial("tcp", addr.IP.String()+":2888")
				if err != nil {
					fmt.Println(err)
				}
				s.electionConn[addr.IP.String()] = &newc
			}
			done += 1
			// fmt.Printf("Knows %s, connected = %d/%d\n", c.RemoteAddr().String(), done, s.nodes-1)
			fmt.Printf("Knows %s \n", c.RemoteAddr().String())
			go s.leaderBroadcast(c)

			// if done == s.nodes-1 { return }
		}

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
	timer := time.NewTimer(4 * time.Second)
	<-timer.C
	if s.waitresponse {
		fmt.Println(s.ip, "message to coordinator timed out") //For debugging/demo purposes
		go s.startelection()
	}
}

//This function is called after the timeout of an expected coordinator response
func (s *Server) startelection() {
	s.electing = true
	s.normal = false
	fmt.Println(s.ip, "Starting Election") //For debugging/demo purposes
	for ip, _ := range s.electionConn {
		go s.sendelection(ip)
	}
}

//This function is run on a separate thread to send the election messages towards all nodes with higher IP
func (s *Server) sendelection(ip string) {
	electionMessage := "COORDINATOR " + s.ip + " EPOCH " + strconv.Itoa(s.highestTxnId.epoch) + " COUNTER " + strconv.Itoa(s.highestTxnId.counter) + "\n"
	fmt.Fprintf(*s.electionConn[ip], electionMessage)
	go s.electiontimer()
}

//This function is run on a separate thread to wait for each sent election message to either receive a response or not. When a response is not received within the time window, the timer will count down the required timeouts
//When the election counter reaches 0, the node will declare victory
func (s *Server) electiontimer() {
	timer := time.NewTimer(2 * time.Second)
	<-timer.C
	if s.electing {
		s.declarevictory()
		s.synctxns()
	}
}

func (s *Server) declarevictory() {
	s.electing = false
	s.normal = true
	fmt.Println(s.ip, "declaring victory") //For debugging/demo purposes
	s.coordinator = s.ip
	s.highestTxnId.epoch = s.highestTxnId.epoch + 1
	s.highestTxnId.counter = 0
	s.role = Leader
	s.leaderConn.Close()
	for ip, _ := range s.listConn {
		c := *s.listConn[ip]
		c.Close()
	}
	go s.connectToPeers()
	time.Sleep(1 * time.Second)
	victoryMessage := "VICTORY " + s.ip + " EPOCH " + strconv.Itoa(s.highestTxnId.epoch) + "\n"
	for ip, _ := range s.electionConn {
		c := *s.electionConn[ip]
		c.Write([]byte(victoryMessage))
	}
}

//This function syncs uncommitted transactions logged by new leader in previous epoch to all followers
func (s *Server) synctxns() {
	s.electing = false
	if len(s.queue) > 0 {
		fmt.Println(s.ip, "syncing transactions") //For debugging/demo purposes
		for _, txn := range s.queue {
			msg := txnid2str(txn.id) + " " + txn.req + "\n"
			for ip, _ := range s.electionConn {
				if s.ip == ip {
					continue
				}
				c := *s.listConn[ip]
				c.Write([]byte("PROPOSE " + msg))
				c.Write([]byte("COMMIT " + msg))
			}
		}
		s.queue = []Transaction{}
	}
	s.normal = true
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
	go s.messageProcess(c)

	for {
		netData, err := bufio.NewReader(c).ReadString('\n')

		if err != nil {
			fmt.Println('c', err)
			return
		}

		temp := strings.TrimSpace(string(netData))
		if temp == "STOP" {
			fmt.Println("Exiting TCP server!")
			break
		}

		s.arr_lock.Lock()
		s.reqQueue = append(s.reqQueue, netData)
		s.arr_lock.Unlock()
	}

}

func (s *Server) messageProcess(c net.Conn) {

	for {
		s.arr_lock.Lock()
		if len(s.reqQueue) == 0 {
			s.arr_lock.Unlock()
			continue
		}

		netData := s.reqQueue[0]
		s.reqQueue = s.reqQueue[1:]
		s.arr_lock.Unlock()
		temp := strings.TrimSpace(string(netData))
		command := strings.Split(temp, " ")
		if command[0] == "make" || command[0] == "delete" || command[0] == "write" {
			if !s.normal {
				reply := "Server busy, retry later\n"
				c.Write([]byte(reply))
			} else if command[0] == "make" {
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
			} else if _, err := os.Stat(command[1] + "/data"); os.IsNotExist(err) {
				fmt.Println("No content in directory")
				reply := "No content in directory\n"
				c.Write([]byte(reply))
			} else {
				s.read_lock.Lock()
				body, err := ioutil.ReadFile(command[1] + "/data")
				if err != nil {
					log.Fatalf("unable to read file: %v", err)
				}
				s.read_lock.Unlock()
				datStr := string(body)
				fmt.Println(datStr)
				c.Write([]byte(datStr + "\n"))
			}
		} else if command[0] == "COORDINATOR" {
			if s.ip == command[1] {
				continue
			}
			fmt.Println("Server", s.ip, "received message:", temp)
			epoch, _ := strconv.Atoi(command[3])
			counter, _ := strconv.Atoi(command[5])
			if epoch < s.highestTxnId.epoch {
				if !s.electing && !s.waiting {
					go s.startelection()
					s.electing = true
				}
			} else if epoch == s.highestTxnId.epoch {
				if counter < s.highestTxnId.counter {
					if !s.electing && !s.waiting {
						go s.startelection()
						s.electing = true
					}
				} else if counter == s.highestTxnId.counter {
					if strings.Compare(s.ip, command[1]) == 1 {
						if !s.electing && !s.waiting {
							go s.startelection()
							s.electing = true
						}
					} else {
						if s.electing {
							s.electing = false
							s.waiting = true
						} else {
							s.waiting = true
						}
					}
				} else {
					if s.electing {
						s.electing = false
						s.waiting = true
					} else {
						s.waiting = true
					}
				}
			} else {
				if s.electing {
					s.electing = false
					s.waiting = true
				} else {
					s.waiting = true
				}
			}
		} else if command[0] == "VICTORY" {
			fmt.Println("Server", s.ip, "received message:", temp)
			s.electing = false
			s.waiting = false
			s.coordinator = command[1]
			epoch, _ := strconv.Atoi(command[3])
			s.highestTxnId.epoch = epoch
			s.highestTxnId.counter = 0
			s.role = Follower
			s.queue = []Transaction{}
			s.leaderConn.Close()
			for ip, _ := range s.listConn {
				c := *s.listConn[ip]
				c.Close()
			}
			go s.connectToPeers()
			if s.timedout != "" {
				time.Sleep(1 * time.Second)
				s.startBroadcast(s.timedout)
			}

		} else if command[0] == "REJOIN" {
			if s.role == "LEADER" {
				fmt.Println("Server", s.ip, "received message:", temp)
				ip := command[1]
				s.handleRejoin(ip)
			} else if s.role == "FOLLOWER" && len(command) > 2 {
				if s.ip == command[3] {
					s.normal = true
				}
			}
		} else if command[0] == "REGISTER" {
			if s.role == "LEADER" {
				//inform coordinator ip
				fmt.Println("Server", s.ip, "received message:", temp)
				ip := command[1]
				newc, err := net.Dial("tcp", ip+":2888")
				if err != nil {
					fmt.Println(err)
				}
				s.electionConn[ip] = &newc
				registerMsg := "REGISTER " + s.ip + " FOR " + ip + "\n"
				newc.Write([]byte(registerMsg))
			} else if s.role == "FOLLOWER" && len(command) > 2 {
				if s.ip == command[3] {
					//set coordinator ip only at rejoiner node
					fmt.Println("Server", s.ip, "received message:", temp)
					s.coordinator = command[1]
					s.waiting = false
					go s.connectToPeers()
					time.Sleep(1 * time.Second)
					rejoinMsg := "REJOIN " + s.ip + "\n"
					c := *s.electionConn[s.coordinator]
					c.Write([]byte(rejoinMsg))
					// s.leaderConn.Write([]byte(rejoinMsg))
				}
			}
		} else {
			reply := "Unrecognised command: " + string(netData)
			c.Write([]byte(reply))
		}
	}
}

func (s *Server) startBroadcast(req string) {
	switch s.role {
	case Follower:
		c := s.leaderConn
		s.timedout = req
		s.waitresponse = true
		go s.messagetimer()
		c.Write([]byte(req + "\n"))

	case Leader:
		if !s.normal {
			break
		}
		temp := strings.Split(req, " ")
		s.executeLeaderBroadcast(req, temp)
	}
}

func newTransaction(id TxnId, req string) Transaction {
	txn := Transaction{
		id:  id,
		req: req,
	}
	return txn
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

		//TODO: are propose and commmits matching? or is the wrong txn being dequeued?
		if temp[0] == "PROPOSE" {
			txnid := strings.Split(temp[1], "-")
			epoch, _ := strconv.Atoi(txnid[0])
			counter, _ := strconv.Atoi(txnid[1])
			if epoch > s.highestTxnId.epoch {
				s.highestTxnId = TxnId{epoch, counter}
			} else if epoch == s.highestTxnId.epoch {
				if counter > s.highestTxnId.counter {
					s.highestTxnId = TxnId{epoch, counter}
				}
			}
			s.timedout = ""
			s.waitresponse = false

			txn := Transaction{
				id:  TxnId{epoch, counter},
				req: strings.Join(temp[2:], " "),
			}
			s.queue = enqueue(s.queue, txn) //enqueue txn
			s.txnlog.Println(bcMsg)
			sendMsg := "ACK"
			sendMsg += " " + strings.Join(temp[1:], " ") + "\n"
			c.Write([]byte(sendMsg))

		} else if temp[0] == "COMMIT" {
			if len(s.queue) > 0 {
				s.queue = s.queue[1:]
			} //dequeue txn
			s.waitresponse = false
			s.txnlog.Println(bcMsg)
			fmt.Println("Committing: " + strings.Join(temp, " "))
			//TODO: respond to client if applicable?
			executeCommand(temp[2:])

		} else if temp[0] == "NODE" {
			newc, err := net.Dial("tcp", temp[1]+":2888")
			if err != nil {
				fmt.Println(err)
				continue
			}
			s.electionConn[temp[1]] = &newc
		}
		bcMsg = ""
		temp = nil
	}
}

/* --- UTILITY FUNCTIONS --- */
func lessThan(a *TxnId, b *TxnId) bool {
	return (a.epoch <= b.epoch && a.counter < b.counter)
}

func enqueue(q []Transaction, txn Transaction) []Transaction {
	q = append(q, txn)
	//priority queue based on TxnId
	if len(q) > 1 {
		sort.Slice(q, func(i, j int) bool {
			var byE, byC bool
			e1 := q[i].id.epoch
			c1 := q[i].id.counter
			e2 := q[j].id.epoch
			c2 := q[j].id.counter
			byE = e1 < e2
			if e1 == e2 {
				byC = c1 < c2
				return byC
			}
			return byE
		})
	}
	return q
}

func txnid2str(id TxnId) string {
	return fmt.Sprintf("%d-%d", id.epoch, id.counter)
}

func str2txnid(str string) TxnId {
	ls := strings.Split(str, "-")
	epoch, _ := strconv.Atoi(ls[0])
	counter, _ := strconv.Atoi(ls[1])
	return TxnId{epoch, counter}
}

/* --- UTILITY FUNCTIONS END --- */

func (s *Server) leaderBroadcast(c net.Conn) {
	reader := bufio.NewReader(c)
	var bcMsg string
	for {
		fwd, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			return
		}
		bcMsg = strings.TrimSpace(string(fwd))

		if !s.normal {
			continue
		}
		temp := strings.Split(bcMsg, " ")
		s.executeLeaderBroadcast(bcMsg, temp)
		bcMsg = ""
		temp = nil
	}
}

func (s *Server) executeLeaderBroadcast(bcMsg string, temp []string) {
	if temp[0] == "ACK" {
		mutex.Lock()
		s.txnReplies[temp[1]] += 1
		replies := s.txnReplies[temp[1]]
		mutex.Unlock()
		//TODO: remove committed txn from map?
		if replies == s.quorum {
			sendMsg := "COMMIT"
			sendMsg += " " + strings.Join(temp[1:], " ") + "\n"
			// c.Write([]byte(sendMsg))
			for i, _ := range s.listConn {
				fc := *s.listConn[i]
				fc.Write([]byte(sendMsg))
			}
			printMsg := strings.TrimSuffix(sendMsg, "\n")
			s.txnlog.Println(printMsg)
			replies = 0
			executeCommand(temp[2:])
		}
	} else if len(temp) > 0 {
		//convert request to transaction
		id := TxnId{s.highestTxnId.epoch, s.highestTxnId.counter + 1}
		s.highestTxnId = id
		mutex.Lock()
		s.txnReplies[txnid2str(id)] = 1
		mutex.Unlock()

		sendMsg := "PROPOSE" + " " + txnid2str(id) + " " + bcMsg + "\n"
		printMsg := strings.TrimSuffix(sendMsg, "\n")
		s.txnlog.Println(printMsg)
		// c.Write([]byte(sendMsg))
		for i, _ := range s.listConn {
			fc := *s.listConn[i]
			fc.Write([]byte(sendMsg))
		}
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

//This function sends uncommitted transactions from current leader to rejoined node
func (s *Server) sendtxns(c net.Conn) {
	if len(s.queue) <= 0 {
		return
	}
	fmt.Println(s.ip, "sending transactions") //For debugging/demo purposes
	for _, txn := range s.queue {
		sendMsg := "PROPOSE" + " " + txnid2str(txn.id) + " " + txn.req + "\n"
		c.Write([]byte(sendMsg))
	}
}

//This function sends all logged messages from current leader to rejoined node
func (s *Server) replaylog(c net.Conn) {
	logCopy := "./copytxn.txt"
	dst, err := os.Create(logCopy)
	if err != nil {
		log.Panic(err)
	}
	defer dst.Close()
	logOriginal := "./mytxn.txt"
	source, err := os.Open(logOriginal)
	if err != nil {
		log.Panic(err)
	}
	defer source.Close()
	nbytes, err := io.Copy(dst, source)
	fmt.Println("Copied bytes", nbytes) //For debugging/demo purposes
	if err != nil {
		log.Panic(err)
	}

	source.Close()
	dst.Close()

	dstr, err := os.Open(logCopy)
	if err != nil {
		log.Panic(err)
	}
	if nbytes == 0 {
		return
	}
	sc := bufio.NewScanner(dstr)
	for sc.Scan() {
		line := sc.Text()
		msg := strings.Split(line, ":")[4]
		msg = strings.TrimSpace(msg)
		c.Write([]byte(msg + "\n"))
	}
	if err := sc.Err(); err != nil {
		log.Panic(err)
	}
}

//This function runs on separate thread and brings rejoined node up-to-date
func (s *Server) handleRejoin(ip string) {
	c := *s.listConn[ip]
	s.replaylog(c)
	s.sendtxns(c)
	msg := "REJOIN " + s.ip + " FOR " + ip + "\n"
	ec := *s.electionConn[ip]
	ec.Write([]byte(msg))
}

//This function is attempt to rejoin network
func (s *Server) rejoin() {
	ADDR_LIST := [...]string{"10.10.10.11", "10.10.10.12", "10.10.10.13", "10.10.10.14"}
	PORT := ":2888"
	msg := "REGISTER " + s.ip + "\n"
	for _, addr := range ADDR_LIST {
		if addr != s.ip {
			newc, err := net.Dial("tcp", addr+PORT)
			if err != nil {
				fmt.Println(err)
			}
			s.electionConn[addr] = &newc
			newc.Write([]byte(msg))
		}
	}
}

func main() {
	localaddr := GetOutboundIP()

	sizeFlag := flag.Int("nodes", 3, "Must be positive integer")
	statusFlag := flag.String("status", "new", "Must be new/rejoin")
	flag.Parse()
	NODE_STATUS := *statusFlag
	NUM_NODES := *sizeFlag
	QUORUM := int(math.Floor(float64(NUM_NODES/2)) + 1)
	LOG_FILE := "./mytxn.txt"

	var coordinator string
	var normal bool
	var txnFile *os.File
	var err error

	switch NODE_STATUS {
	case "new":
		txnFile, err = os.OpenFile(LOG_FILE, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			log.Panic(err)
		}
		defer txnFile.Close()
		coordinator = "10.10.10.14"
		normal = true

	case "rejoin":
		txnFile, err = os.OpenFile(LOG_FILE, os.O_TRUNC|os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			log.Panic(err)
		}
		defer txnFile.Close()
		normal = false
	}

	server := &Server{
		ip:            localaddr,
		coordinator:   coordinator,
		normal:        normal,
		listeningport: ":2888",
		electing:      false,
		waiting:       false,
		waitresponse:  false,
		listConn:      make(map[string]*net.Conn),
		myChannel:     make(chan string),
		electionConn:  make(map[string]*net.Conn),
		highestTxnId:  TxnId{0, 0},
		quorum:        QUORUM,
		txnFile:       txnFile,
		nodes:         NUM_NODES,
		queue:         []Transaction{},
		txnReplies:    make(map[string]int),
		txnlog:        log.New(txnFile, "", log.Ldate|log.Ltime|log.Lshortfile),
	}

	if localaddr == coordinator {
		server.role = Leader
	} else {
		server.role = Follower
	}

	if NODE_STATUS == "new" {
		go server.connectToPeers()
	} else {
		go server.rejoin()
	}

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
		fmt.Printf("Received new connection from ")
		var connip string
		if addr, ok := c.RemoteAddr().(*net.TCPAddr); ok {
			fmt.Println(addr.IP.String())
			connip = addr.IP.String()
		}
		if server.ip == connip {
			c.Close()
			continue
		}
		go server.handleConnection(c)
	}
}
