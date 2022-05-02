From ubuntu:latest
MAINTAINER joshd joshuadean282@gmail.com

RUN apt-get -y update
RUN apt-get -y upgrade
RUN apt-get install -y build-essential


RUN DEBIAN_FRONTEND=noninteractive apt-get -y install binutils curl iproute2 iputils-ping nano net-tools mtr-tiny netcat openbsd-inetd tcpdump telnet telnetd 

RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt-get -y install tzdata
RUN apt-get install -y golang
RUN apt-get install -y python3-pip
