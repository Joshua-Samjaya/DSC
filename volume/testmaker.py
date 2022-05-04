total = 1000

f = open("volume/commands_write.txt", "w")
string = ""
for i in range(total):
    string+="write znode1 "+str(i)+"\n"

f.write(string)
f.close()

f = open("volume/commands_read.txt", "w")
string = ""
for i in range(total):
    string+="read znode1"+"\n"

f.write(string)
f.close()

f = open("volume/commands_write2.txt", "w")
string = ""
for i in range(total):
    string+="write znode1 -"+str(i)+"\n"

f.write(string)
f.close()
