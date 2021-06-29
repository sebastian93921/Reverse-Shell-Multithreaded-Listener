package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"os/signal"
)

var signalChan = make(chan os.Signal, 1)
var backgroundCommand = "rev-bg"

func main() {
	fmt.Println("===========================")
	fmt.Println(" Reverse Shell listener")
	fmt.Println("===========================")

	// Keyboard signal notify
	signal.Notify(signalChan, os.Interrupt)

	var destinationPort string
	clients := map[int]*Socket{}

	flag.Parse()
	if flag.NFlag() == 0 && flag.NArg() == 0 {
		fmt.Println("Usage: reverseshell-listener <port>")
		os.Exit(1)
	}

	if _, err := strconv.Atoi(flag.Arg(0)); err != nil {
		fmt.Println("[!] Port cannot be empty and not an integer")
		os.Exit(1)
	} else {
		destinationPort = fmt.Sprintf(":%v", flag.Arg(0))
	}

	fmt.Println("[+] Listening on port", destinationPort)
	go connectionThread(destinationPort, clients)

	reader := bufio.NewReader(os.Stdin)
	connectedSession := 1
	for {
		select {
		case <-signalChan:
			fmt.Println("\n[+] Application quit successfully")
			os.Exit(0)
		default:
			if len(clients) > 0 && connectedSession == 0 {
				fmt.Print("listener> ")
				text, _ := reader.ReadString('\n')
				connectedSession = commandHandler(text, clients)
			}else if len(clients) > 0 && connectedSession != 0 {
				if !clients[connectedSession].isClosed {
					clients[connectedSession].interact()
				}else{
					fmt.Println("[-] No matched session or session has been closed")
				}
				connectedSession = 0
			}
			time.Sleep(1 * time.Microsecond)
		}
		
	}
}

func commandHandler(cmd string, clients map[int]*Socket) int{
	connectedSession := 0

	splitCommand := strings.Split(cmd," ")
	switch strings.TrimSuffix(splitCommand[0], "\n") {
	case "help":
		fmt.Println("sessions \t- List sessions")
		fmt.Println("session <num> \t- Get into session by ID")
	case "sessions":
		fmt.Println("---------------------->")
		for _, client := range clients {
				fmt.Println(client.status())
		}
		fmt.Println("<----------------------")
	case "session":
		connectedSession, _ = strconv.Atoi(strings.TrimSuffix(splitCommand[1], "\n"))
		if connectedSession > len(clients) {
			fmt.Println("[!] Wrong session selected")
			connectedSession = 0
		}
	}

	return connectedSession
}


func connectionThread(destPort string, clients map[int]*Socket){
	listener, err := net.Listen("tcp", destPort)
	if err != nil {	
		fmt.Println("[-]", err)
	}
	//Assign session ID
	sessionId := 1
	for {
		// Listen for an incoming connection.
		con, err := listener.Accept()
		if err != nil {
			fmt.Println("[-] Error accepting:", err)
		}
		// Handle connections in a new goroutine.
		fmt.Println("[+] Got connection from <", con.RemoteAddr().String(),">, Session ID:", sessionId)
		socket := &Socket{sessionId: sessionId ,con: con}
		clients[sessionId] = socket
		sessionId = sessionId + 1
	}
}


/*
	Socket
*/
type Socket struct {
	sessionId int
	con net.Conn
	isBackground bool
	isClosed bool
}

func (s *Socket) interact() {
	s.isBackground = false

	fmt.Printf("[+] Interact with Session ID: %d \n", s.sessionId)
	fmt.Printf("[!] Type '%s' to background the current session\n", backgroundCommand)
	fmt.Println("[+] Happy cracking!")
	// Mark two signal for informational
	stdoutThread := s.copyFromConnection(s.con, os.Stdout)
	stdinThread := s.readingFromStdin(os.Stdin, s.con)
	select {
	case <-stdoutThread:
		fmt.Println("[-] Remote connection is closed")
	case <-stdinThread:
		// fmt.Println("[-] DEBUG: Terminated by user",stdinThread)
	}
}

func (s *Socket) copyFromConnection(src io.Reader, dst io.Writer) <-chan int {
	buf := make([]byte, 1024)
	sync_channel := make(chan int)
	go func() {
		// Defer handling
		defer func() {
			if con, ok := dst.(net.Conn); ok {
				if s.isClosed {
					con.Close()
					fmt.Println("[-] Connection closed:", con.RemoteAddr())
				}
			}
			// Notify that processing is finished
			sync_channel <- 0
		}()
		for {
			var nBytes int
			var err error
			nBytes, err = src.Read(buf)
			if s.isBackground || s.isClosed {
				break
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println("[!] Read error:", err)
					s.isClosed = true
				}
				break
			}
			_, err = dst.Write(buf[0:nBytes])
			if err != nil {
				fmt.Println("[!] Write error:", err)
				s.isClosed = true
			}
		}
	}()
	return sync_channel
}


func (s *Socket) readingFromStdin(src io.Reader, dst io.Writer) <-chan int {
	buf := make([]byte, 1024)
	sync_channel := make(chan int)
	go func() {
		// Defer handling
		defer func() {
			if con, ok := dst.(net.Conn); ok {
				if s.isClosed {
					con.Close()
					fmt.Println("[-] Connection killed:", con.RemoteAddr())
				}else{
					fmt.Println("[-] Backgrounded session",s.sessionId,", status:",s.isBackground)
				}
			}
			// Notify that processing is finished
			sync_channel <- 0
		}()
		for {
			var nBytes int
			var err error
			nBytes, err = src.Read(buf)
			// Special command
			command := string(buf[0:nBytes - 1])
			if command == backgroundCommand {
				s.isBackground = true
			}

			if s.isBackground || s.isClosed {
				break
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println("[!] Read error:", err)
					s.isClosed = true
				}
				break
			}
			// Process input
			select {
			case <-signalChan:
				// Ctrl+C handle
				result := prompt("[+] Do you really want to quit?", src)
				if result {
					s.isClosed = true
					break
				}else{
					_, err = dst.Write([]byte("\003\n"))
				}
			default:
				_, err = dst.Write(buf[0:nBytes])
			}
			if err != nil {
				fmt.Println("[!] Write error:", err)
				s.isClosed = true
			}
		}
	}()
	return sync_channel
}

func (s *Socket) status() string{
	return fmt.Sprintf("Session ID: [%d], Connection <%s> Seesion killed [%t]", s.sessionId, s.con.RemoteAddr(), s.isClosed)
}


func prompt(message string, src io.Reader) bool{
	buf := make([]byte, 1024)
	for{
		fmt.Print(message + " (Y/N): ")
		nBytes, _ := src.Read(buf)
		input := strings.TrimSuffix(string(buf[0:nBytes]), "\n")
		input = strings.ToUpper(input)
		if input == "Y" || input == "N" {
			return input == "Y"
		}
	}
}