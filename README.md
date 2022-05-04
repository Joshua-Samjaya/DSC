
# Distributed Systems Project - Zookeeper as a File System

## Introduction
This repository contains a working implementation of lightweight Zookeepeer as a file storage. The infrastructure is build on top of Docker to simulate multiple Zookeeper servers and clients. The current architecture is as follows:
1. Node1 to node3 as servers (Node3 as initial leader)
2. Node4 to node8 as clients

The implementation support the following requests
1. **make <dir_name>**                  : make a directory with the name dir_name
2. **delete  <dir_name>**               : delete the directory with name dir_name
3. **write <dir_name> <dir_content>**   : overwrite the content of dir_name with dir_content 
4. **read <dir_name>**                  : read the content of dir_name

The implementation supports concurrent operation from clients, where multiple clients can read/write to different servers concurrently. The implementation guarantees FIFO write order per individual client + liveliness to serve the requests from all clients. 

If a server crash, the servers will continue operating as long as the number of alive servers is greater than the quorum size. If the crashed server is the leader, start election and elect new leader before continuing process as normal. If a crashed server rejoins, it will commit all logs from the current leader and continue operating, but it will not trigger another election.

## Documentation
There are three scripts, which are:
1. **server.go**        : the server-side script
2. **client.go**     : the client-side script; handles one manually-inputted request at a time
3. **testclient.go**   : the client-side testing script; handle multiple requests from a specified command input file

To run the code, follow these steps:
1. Build and connect to all containers
2. For the server nodes, run the following from main dir: ***go run home/volume/server.go***. ***Important:*** Run node3 first as it is the initial leader.
3. For the client nodes, run ***cd home/volume*** followed by ***go run client.go***. Specify the server node to connect to by inputting its IP.
4. Input requests from client to server

To test the code, follow these steps:
1. Build and connect to all containers
2. For the server nodes, run the following from main dir: ***go run home/volume/server.go***. It is important to run node3 first as it is the initial leader.
3. For the client nodes, run ***cd home/volume*** followed by ***go run testclient.go <path_to_command_file>***. Specify the server node to connect to by inputting its IP.
4. The script will automatically sent requests to the server at a rate of 10 requests per second.


